// Package kubernetes implements runtime.Runtime with client-go (spec sections
// 8.2 and 8.3). Every sandbox is a pod in the sandboxes namespace, created
// locked down (spec sections 13.1 and 13.2): its own user namespace
// (hostUsers false), seccomp RuntimeDefault, all capabilities dropped and the
// exercise's allowlist added back, no privilege escalation, no service account
// token, no host paths, memory and CPU limits, and no cluster DNS. The pause
// container is PID 1 and reaps stopped processes, like docker's --init.
//
// The default-deny NetworkPolicy, the ResourceQuota and the LimitRange of the
// namespace live in deploy/k8s, not here.
package kubernetes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	utilexec "k8s.io/client-go/util/exec"

	"github.com/DIZAMU/RootCause/internal/runtime"
)

// Namespace holds every sandbox pod.
const Namespace = "sandboxes"

const (
	// container is the name of the one container in a sandbox pod.
	container = "sandbox"
	// labelSandbox marks every pod this package creates, so List and the
	// reaper find RootCause sandboxes and nothing else.
	labelSandbox = "rootcause.sandbox"
	// labelNetwork is the spec's network mode ("none" or "internal"). The
	// NetworkPolicies in deploy/k8s select on it.
	labelNetwork = "rootcause.network"
)

// deleteTimeout bounds how long Destroy waits for a pod to be gone, so a
// sandbox can be created again under the same name right after.
const deleteTimeout = 60 * time.Second

// ErrRestartUnsupported means the runtime cannot restart a sandbox and keep
// its files: a restarted container starts again from its image.
var ErrRestartUnsupported = errors.New("the kubernetes runtime cannot restart a sandbox and keep its files")

// Runtime controls sandbox pods.
type Runtime struct {
	cs        kubernetes.Interface
	config    *rest.Config
	ns        string
	inCluster bool
}

var _ runtime.Runtime = (*Runtime)(nil)
var _ runtime.Diagnoser = (*Runtime)(nil)
var _ runtime.SystemdChecker = (*Runtime)(nil)
var _ runtime.Terminal = (*Runtime)(nil)

// New returns a runtime for the cluster it runs in, or, outside a cluster, for
// the current kubeconfig context.
func New() (*Runtime, error) {
	config, err := rest.InClusterConfig()
	inCluster := err == nil
	if errors.Is(err, rest.ErrNotInCluster) {
		rules := clientcmd.NewDefaultClientConfigLoadingRules()
		config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{}).ClientConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("kubernetes config: %w", err)
	}
	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("kubernetes client: %w", err)
	}
	return &Runtime{cs: cs, config: config, ns: Namespace, inCluster: inCluster}, nil
}

// InCluster reports whether the runtime uses the pod's own service account.
func (r *Runtime) InCluster() bool { return r.inCluster }

// Create starts a locked-down sandbox pod and returns it once its container
// runs, as docker run -d does. The engine then waits until the sandbox
// accepts commands. A pod that cannot start (a missing image, say) is deleted
// and the error describes it.
//
// Pods on the internal network get a hosts entry for every other internal pod
// that has an address, so a check runner reaches its sandbox by name, as on
// docker's internal network. Which of them may actually connect is up to the
// NetworkPolicies.
func (r *Runtime) Create(ctx context.Context, spec runtime.SandboxSpec) (runtime.Sandbox, error) {
	if spec.Name == "" || spec.Image == "" {
		return runtime.Sandbox{}, fmt.Errorf("sandbox spec needs a name and an image")
	}
	var aliases []corev1.HostAlias
	if spec.Network == "internal" {
		pods, err := r.cs.CoreV1().Pods(r.ns).List(ctx, metav1.ListOptions{
			LabelSelector: labelSandbox + "=true," + labelNetwork + "=internal",
		})
		if err != nil {
			return runtime.Sandbox{}, fmt.Errorf("list internal pods: %w", err)
		}
		aliases = hostAliases(pods.Items)
	}
	pod, err := podFor(spec, r.ns, aliases)
	if err != nil {
		return runtime.Sandbox{}, err
	}
	created, err := r.cs.CoreV1().Pods(r.ns).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return runtime.Sandbox{}, fmt.Errorf("create pod %s: %w", spec.Name, err)
	}
	if err := r.waitRunning(ctx, created.Name); err != nil {
		_ = r.Destroy(context.Background(), created.Name)
		return runtime.Sandbox{}, err
	}
	return runtime.Sandbox{
		ID:      created.Name,
		Name:    created.Name,
		Image:   spec.Image,
		Created: created.CreationTimestamp.Time,
		Labels:  spec.Labels,
	}, nil
}

// startTimeout bounds how long a new pod may take to run its container.
const startTimeout = 60 * time.Second

// waitRunning polls the pod until its container runs. It fails at once on a
// pod that has ended or a container that cannot start, and after
// startTimeout otherwise, with Diagnose's description of the pod.
func (r *Runtime) waitRunning(ctx context.Context, name string) error {
	ctx, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()
	for {
		pod, err := r.cs.CoreV1().Pods(r.ns).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			switch started(pod) {
			case podRunning:
				return nil
			case podFailed:
				return fmt.Errorf("pod %s did not start: %s", name, r.diagnose(context.Background(), name))
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("pod %s not running within %s: %s", name, startTimeout, r.diagnose(context.Background(), name))
		case <-time.After(200 * time.Millisecond):
		}
	}
}

type podStart int

const (
	podStarting podStart = iota
	podRunning
	podFailed
)

// cannotStart are the waiting reasons of a container that will not start
// without a change to the pod or the node.
var cannotStart = map[string]bool{
	"ErrImageNeverPull":          true,
	"InvalidImageName":           true,
	"CreateContainerConfigError": true,
	"CreateContainerError":       true,
	"RunContainerError":          true,
}

// started reads a pod's progress towards a running container.
func started(pod *corev1.Pod) podStart {
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		return podFailed
	}
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name != container {
			continue
		}
		switch {
		case cs.State.Running != nil:
			return podRunning
		case cs.State.Terminated != nil:
			return podFailed
		case cs.State.Waiting != nil && cannotStart[cs.State.Waiting.Reason]:
			return podFailed
		}
	}
	return podStarting
}

// hostAliases maps the name of every running pod with an address to that
// address, sorted by name.
func hostAliases(pods []corev1.Pod) []corev1.HostAlias {
	var aliases []corev1.HostAlias
	for _, p := range pods {
		if p.Status.PodIP == "" || p.DeletionTimestamp != nil {
			continue
		}
		aliases = append(aliases, corev1.HostAlias{IP: p.Status.PodIP, Hostnames: []string{p.Name}})
	}
	sort.Slice(aliases, func(i, j int) bool { return aliases[i].Hostnames[0] < aliases[j].Hostnames[0] })
	return aliases
}

// podFor builds the pod for a sandbox spec.
func podFor(spec runtime.SandboxSpec, ns string, aliases []corev1.HostAlias) (*corev1.Pod, error) {
	no, yes := false, true
	labels := map[string]string{labelSandbox: "true", labelNetwork: networkLabel(spec.Network)}
	for k, v := range spec.Labels {
		labels[k] = v
	}

	limits := corev1.ResourceList{}
	if spec.Memory != "" {
		q, err := resource.ParseQuantity(spec.Memory)
		if err != nil {
			return nil, fmt.Errorf("memory %q: %w", spec.Memory, err)
		}
		limits[corev1.ResourceMemory] = q
	}
	if spec.CPUs > 0 {
		limits[corev1.ResourceCPU] = *resource.NewMilliQuantity(int64(spec.CPUs*1000), resource.DecimalSI)
	}

	var volumes []corev1.Volume
	var mounts []corev1.VolumeMount
	for i, t := range spec.Tmpfs {
		size, err := resource.ParseQuantity(t.Size)
		if err != nil {
			return nil, fmt.Errorf("tmpfs %s size %q: %w", t.Path, t.Size, err)
		}
		name := fmt.Sprintf("tmpfs-%d", i)
		volumes = append(volumes, corev1.Volume{Name: name, VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory, SizeLimit: &size},
		}})
		mounts = append(mounts, corev1.VolumeMount{Name: name, MountPath: t.Path})
	}

	caps := make([]corev1.Capability, len(spec.Caps))
	for i, c := range spec.Caps {
		caps[i] = corev1.Capability(c)
	}
	grace := int64(0)

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: spec.Name, Namespace: ns, Labels: labels},
		Spec: corev1.PodSpec{
			HostUsers:                     &no,
			AutomountServiceAccountToken:  &no,
			EnableServiceLinks:            &no,
			ShareProcessNamespace:         &yes,
			RestartPolicy:                 corev1.RestartPolicyNever,
			TerminationGracePeriodSeconds: &grace,
			// No cluster DNS: the sandbox resolves nothing it is not given,
			// and learns nothing about the cluster from resolv.conf.
			DNSPolicy:   corev1.DNSNone,
			DNSConfig:   &corev1.PodDNSConfig{Nameservers: []string{"127.0.0.1"}},
			HostAliases: aliases,
			SecurityContext: &corev1.PodSecurityContext{
				SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
			},
			Containers: []corev1.Container{{
				Name:            container,
				Image:           spec.Image,
				ImagePullPolicy: corev1.PullNever,
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: &no,
					Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}, Add: caps},
				},
				Resources:    corev1.ResourceRequirements{Limits: limits, Requests: limits},
				VolumeMounts: mounts,
			}},
			Volumes: volumes,
		},
	}, nil
}

// networkLabel maps a spec network to the pod's network label. Anything but
// "internal" is "none", as in the docker runtime.
func networkLabel(network string) string {
	if network == "internal" {
		return "internal"
	}
	return "none"
}

// CheckSystemd always returns runtime.ErrSystemdUnsupported: in a pod with
// its own user namespace the cgroup filesystem stays owned by host root, so
// systemd cannot create its own cgroups and does not boot.
func (r *Runtime) CheckSystemd(context.Context) error {
	return runtime.ErrSystemdUnsupported
}

// Exec runs a command in the sandbox and captures its output and exit code.
// A non-zero exit from the command is returned in ExecResult.ExitCode, not as
// a Go error; a Go error means the exec itself could not run.
func (r *Runtime) Exec(ctx context.Context, id string, cmd []string, stdin io.Reader) (runtime.ExecResult, error) {
	var stdout, stderr bytes.Buffer
	err := r.stream(ctx, id, cmd, remotecommand.StreamOptions{Stdin: stdin, Stdout: &stdout, Stderr: &stderr})
	res := runtime.ExecResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if err == nil {
		return res, nil
	}
	var ee utilexec.ExitError
	if errors.As(err, &ee) && ee.Exited() {
		res.ExitCode = ee.ExitStatus()
		return res, nil
	}
	return res, fmt.Errorf("exec in pod %s: %w", id, err)
}

// Terminal runs a command in the sandbox with a terminal, for the
// dashboard's web terminal.
func (r *Runtime) Terminal(ctx context.Context, id string, cmd []string, stdin io.Reader, stdout io.Writer, sizes <-chan runtime.TermSize) error {
	return r.stream(ctx, id, cmd, remotecommand.StreamOptions{
		Stdin: stdin, Stdout: stdout, Tty: true, TerminalSizeQueue: sizeQueue(sizes),
	})
}

// sizeQueue hands terminal sizes to remotecommand until the channel closes.
type sizeQueue <-chan runtime.TermSize

func (q sizeQueue) Next() *remotecommand.TerminalSize {
	s, ok := <-q
	if !ok {
		return nil
	}
	return &remotecommand.TerminalSize{Width: s.Cols, Height: s.Rows}
}

// stream runs one exec through the API server's pods/exec subresource, over
// WebSocket.
func (r *Runtime) stream(ctx context.Context, id string, cmd []string, opts remotecommand.StreamOptions) error {
	req := r.cs.CoreV1().RESTClient().Post().
		Resource("pods").Namespace(r.ns).Name(id).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   cmd,
			Stdin:     opts.Stdin != nil,
			Stdout:    opts.Stdout != nil,
			Stderr:    opts.Stderr != nil,
			TTY:       opts.Tty,
		}, scheme.ParameterCodec)
	exec, err := remotecommand.NewWebSocketExecutor(r.config, "GET", req.URL().String())
	if err != nil {
		return err
	}
	return exec.StreamWithContext(ctx, opts)
}

// CopyIn writes data to a path inside the sandbox. Kubernetes has no copy
// API, so the data goes to `cat` on stdin; the file is made world-readable,
// as docker cp leaves it.
func (r *Runtime) CopyIn(ctx context.Context, id string, path string, data []byte) error {
	res, err := r.Exec(ctx, id, []string{"/bin/sh", "-c", `cat > "$1" && chmod 0644 "$1"`, "sh", path}, bytes.NewReader(data))
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("copy to %s in pod %s: exit %d: %s", path, id, res.ExitCode, strings.TrimSpace(string(res.Stderr)))
	}
	return nil
}

// Restart returns ErrRestartUnsupported. The only exercises that ask for a
// restart before the check boot systemd, which CheckSystemd refuses first.
func (r *Runtime) Restart(context.Context, string) error {
	return ErrRestartUnsupported
}

// Destroy deletes the sandbox pod at once and waits until it is gone. It is
// not an error if it is already gone.
func (r *Runtime) Destroy(ctx context.Context, id string) error {
	grace := int64(0)
	err := r.cs.CoreV1().Pods(r.ns).Delete(ctx, id, metav1.DeleteOptions{GracePeriodSeconds: &grace})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete pod %s: %w", id, err)
	}
	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()
	for {
		_, err := r.cs.CoreV1().Pods(r.ns).Get(ctx, id, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("pod %s still exists after %s", id, deleteTimeout)
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// List returns the RootCause sandbox pods that are not being deleted, with
// their labels.
func (r *Runtime) List(ctx context.Context) ([]runtime.Sandbox, error) {
	pods, err := r.cs.CoreV1().Pods(r.ns).List(ctx, metav1.ListOptions{LabelSelector: labelSandbox + "=true"})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}
	var boxes []runtime.Sandbox
	for _, p := range pods.Items {
		if p.DeletionTimestamp != nil {
			continue
		}
		image := ""
		if len(p.Spec.Containers) > 0 {
			image = p.Spec.Containers[0].Image
		}
		boxes = append(boxes, runtime.Sandbox{
			ID: p.Name, Name: p.Name, Image: image, Created: p.CreationTimestamp.Time, Labels: p.Labels,
		})
	}
	return boxes, nil
}

// diagnoseLogLines is how many of a sandbox's last log lines Diagnose shows.
const diagnoseLogLines = 20

// Diagnose describes a sandbox pod for an error message: its phase, why its
// container is waiting or how it ended, and its last log lines. It never
// fails; what it cannot read, it says it cannot read.
func (r *Runtime) Diagnose(ctx context.Context, id string) string {
	return r.diagnose(ctx, id)
}

// diagnose is Diagnose with its own short deadline, for callers whose context
// may have ended.
func (r *Runtime) diagnose(ctx context.Context, id string) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pod, err := r.cs.CoreV1().Pods(r.ns).Get(ctx, id, metav1.GetOptions{})
	if err != nil {
		return "state unknown: " + err.Error()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "pod phase %s", pod.Status.Phase)
	for _, cs := range pod.Status.ContainerStatuses {
		switch {
		case cs.State.Waiting != nil:
			fmt.Fprintf(&b, ", waiting: %s %s", cs.State.Waiting.Reason, cs.State.Waiting.Message)
		case cs.State.Terminated != nil:
			fmt.Fprintf(&b, ", exit code %d (%s)", cs.State.Terminated.ExitCode, cs.State.Terminated.Reason)
		}
	}
	lines := int64(diagnoseLogLines)
	logs, err := r.cs.CoreV1().Pods(r.ns).GetLogs(id, &corev1.PodLogOptions{Container: container, TailLines: &lines}).DoRaw(ctx)
	tail := strings.TrimSpace(string(logs))
	switch {
	case err != nil:
		b.WriteString("; logs unavailable")
	case tail == "":
		b.WriteString("; no log output")
	default:
		fmt.Fprintf(&b, "; last log lines:\n%s", tail)
	}
	return b.String()
}
