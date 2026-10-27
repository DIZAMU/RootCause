package kubernetes

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/DIZAMU/RootCause/internal/runtime"
)

func TestPodForLocksDown(t *testing.T) {
	spec := runtime.SandboxSpec{
		Name: "rootcause-s-1", Image: "rootcause/perm-01-shared-folder:v1",
		Memory: "256Mi", CPUs: 0.5, Network: "none",
		Caps:   []string{"CHOWN", "SETUID"},
		Labels: map[string]string{"rootcause.session": "s-1"},
		Tmpfs:  []runtime.Tmpfs{{Path: "/var/spool", Size: "32Mi"}},
	}
	pod, err := podFor(spec, Namespace, nil)
	if err != nil {
		t.Fatal(err)
	}
	ps := pod.Spec
	if ps.HostUsers == nil || *ps.HostUsers {
		t.Error("hostUsers must be false: pod root must not be host root")
	}
	if ps.AutomountServiceAccountToken == nil || *ps.AutomountServiceAccountToken {
		t.Error("the service account token must not be mounted")
	}
	if ps.EnableServiceLinks == nil || *ps.EnableServiceLinks {
		t.Error("service links must be off: no cluster addresses in the environment")
	}
	if ps.ShareProcessNamespace == nil || !*ps.ShareProcessNamespace {
		t.Error("shareProcessNamespace must be on, so pause reaps stopped processes")
	}
	if ps.SecurityContext == nil || ps.SecurityContext.SeccompProfile == nil ||
		ps.SecurityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Error("seccomp must be RuntimeDefault")
	}
	if ps.HostNetwork || ps.HostPID || ps.HostIPC {
		t.Error("no host namespaces")
	}
	if ps.DNSPolicy != corev1.DNSNone || ps.DNSConfig == nil || !reflect.DeepEqual(ps.DNSConfig.Nameservers, []string{"127.0.0.1"}) {
		t.Errorf("DNS: policy %q config %+v, want None with 127.0.0.1 only", ps.DNSPolicy, ps.DNSConfig)
	}
	if ps.RestartPolicy != corev1.RestartPolicyNever {
		t.Errorf("restartPolicy %q, want Never", ps.RestartPolicy)
	}
	for _, v := range ps.Volumes {
		if v.HostPath != nil {
			t.Errorf("volume %s is a hostPath", v.Name)
		}
	}
	if len(ps.Containers) != 1 {
		t.Fatalf("%d containers, want 1", len(ps.Containers))
	}
	c := ps.Containers[0]
	sc := c.SecurityContext
	if sc == nil || sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
		t.Error("allowPrivilegeEscalation must be false")
	}
	if sc.Privileged != nil && *sc.Privileged {
		t.Error("privileged")
	}
	if !reflect.DeepEqual(sc.Capabilities.Drop, []corev1.Capability{"ALL"}) ||
		!reflect.DeepEqual(sc.Capabilities.Add, []corev1.Capability{"CHOWN", "SETUID"}) {
		t.Errorf("capabilities %+v, want drop ALL, add the exercise's", sc.Capabilities)
	}
	if c.ImagePullPolicy != corev1.PullNever {
		t.Errorf("imagePullPolicy %q, want Never", c.ImagePullPolicy)
	}
	if got := c.Resources.Limits[corev1.ResourceMemory]; got.Cmp(resource.MustParse("256Mi")) != 0 {
		t.Errorf("memory limit %s", got.String())
	}
	if got := c.Resources.Limits[corev1.ResourceCPU]; got.MilliValue() != 500 {
		t.Errorf("cpu limit %s", got.String())
	}
	if len(ps.Volumes) != 1 || ps.Volumes[0].EmptyDir == nil || ps.Volumes[0].EmptyDir.Medium != corev1.StorageMediumMemory ||
		ps.Volumes[0].EmptyDir.SizeLimit.Cmp(resource.MustParse("32Mi")) != 0 || c.VolumeMounts[0].MountPath != "/var/spool" {
		t.Errorf("tmpfs: volumes %+v mounts %+v", ps.Volumes, c.VolumeMounts)
	}
	want := map[string]string{labelSandbox: "true", labelNetwork: "none", "rootcause.session": "s-1"}
	if !reflect.DeepEqual(pod.Labels, want) {
		t.Errorf("labels %v, want %v", pod.Labels, want)
	}
}

func TestPodForRejectsBadSizes(t *testing.T) {
	if _, err := podFor(runtime.SandboxSpec{Name: "a", Image: "i", Memory: "lots"}, Namespace, nil); err == nil {
		t.Error("memory \"lots\" accepted")
	}
	if _, err := podFor(runtime.SandboxSpec{Name: "a", Image: "i", Tmpfs: []runtime.Tmpfs{{Path: "/x", Size: "big"}}}, Namespace, nil); err == nil {
		t.Error("tmpfs size \"big\" accepted")
	}
}

func TestNetworkLabel(t *testing.T) {
	for in, want := range map[string]string{"internal": "internal", "none": "none", "": "none", "host": "none"} {
		if got := networkLabel(in); got != want {
			t.Errorf("networkLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func pod(name, ip string, deleting bool, labels map[string]string) *corev1.Pod {
	p := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: Namespace, Labels: labels}}
	p.Status.PodIP = ip
	if deleting {
		now := metav1.Now()
		p.DeletionTimestamp = &now
	}
	return p
}

func TestHostAliases(t *testing.T) {
	got := hostAliases([]corev1.Pod{
		*pod("b", "10.0.0.2", false, nil),
		*pod("a", "10.0.0.1", false, nil),
		*pod("pending", "", false, nil),
		*pod("going", "10.0.0.3", true, nil),
	})
	want := []corev1.HostAlias{{IP: "10.0.0.1", Hostnames: []string{"a"}}, {IP: "10.0.0.2", Hostnames: []string{"b"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("hostAliases = %+v, want %+v", got, want)
	}
}

func TestStarted(t *testing.T) {
	state := func(s corev1.ContainerState) *corev1.Pod {
		p := pod("p", "", false, nil)
		p.Status.Phase = corev1.PodPending
		p.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: container, State: s}}
		return p
	}
	cases := []struct {
		name string
		pod  *corev1.Pod
		want podStart
	}{
		{"no status yet", pod("p", "", false, nil), podStarting},
		{"creating", state(corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}}), podStarting},
		{"running", state(corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}), podRunning},
		{"image missing", state(corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ErrImageNeverPull"}}), podFailed},
		{"exited", state(corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1}}), podFailed},
	}
	for _, c := range cases {
		if got := started(c.pod); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	failed := pod("p", "", false, nil)
	failed.Status.Phase = corev1.PodFailed
	if started(failed) != podFailed {
		t.Error("failed pod not reported as failed")
	}
}

// fakeRuntime is a Runtime over client-go's fake clientset, where every
// created pod runs at once and gets the next address.
func fakeRuntime(t *testing.T, objects ...k8sruntime.Object) (*Runtime, *fake.Clientset) {
	t.Helper()
	cs := fake.NewClientset(objects...)
	ip := 10
	cs.PrependReactor("create", "pods", func(a k8stesting.Action) (bool, k8sruntime.Object, error) {
		p := a.(k8stesting.CreateAction).GetObject().(*corev1.Pod)
		ip++
		p.Status.PodIP = "10.42.1." + strconv.Itoa(ip)
		p.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: container, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}}
		return false, nil, nil
	})
	return &Runtime{cs: cs, ns: Namespace}, cs
}

func TestCreateGivesInternalPodsTheirPeers(t *testing.T) {
	ctx := context.Background()
	internal := map[string]string{labelSandbox: "true", labelNetwork: "internal"}
	isolated := map[string]string{labelSandbox: "true", labelNetwork: "none"}
	r, cs := fakeRuntime(t,
		pod("rootcause-s-1", "10.42.0.5", false, internal),
		pod("rootcause-s-2", "10.42.0.6", false, isolated),
	)

	sb, err := r.Create(ctx, runtime.SandboxSpec{Name: "rootcause-s-1-runner", Image: "img", Network: "internal"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := cs.CoreV1().Pods(Namespace).Get(ctx, sb.ID, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := []corev1.HostAlias{{IP: "10.42.0.5", Hostnames: []string{"rootcause-s-1"}}}
	if !reflect.DeepEqual(got.Spec.HostAliases, want) {
		t.Errorf("runner hostAliases %+v, want only the internal sandbox %+v", got.Spec.HostAliases, want)
	}

	sb, err = r.Create(ctx, runtime.SandboxSpec{Name: "rootcause-s-3", Image: "img", Network: "none"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ = cs.CoreV1().Pods(Namespace).Get(ctx, sb.ID, metav1.GetOptions{})
	if len(got.Spec.HostAliases) != 0 {
		t.Errorf("isolated pod got hostAliases %+v", got.Spec.HostAliases)
	}
}

func TestDestroyAndList(t *testing.T) {
	ctx := context.Background()
	labels := map[string]string{labelSandbox: "true", labelNetwork: "none", "rootcause.session": "s-1"}
	r, _ := fakeRuntime(t,
		pod("rootcause-s-1", "10.42.0.5", false, labels),
		pod("rootcause-s-2", "10.42.0.6", true, map[string]string{labelSandbox: "true"}),
		pod("not-ours", "10.42.0.7", false, nil),
	)
	boxes, err := r.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(boxes) != 1 || boxes[0].ID != "rootcause-s-1" || boxes[0].Labels["rootcause.session"] != "s-1" {
		t.Errorf("List = %+v, want only rootcause-s-1 (not the deleting pod, not foreign pods)", boxes)
	}
	if err := r.Destroy(ctx, "rootcause-s-1"); err != nil {
		t.Fatal(err)
	}
	if err := r.Destroy(ctx, "rootcause-s-1"); err != nil {
		t.Errorf("Destroy of a missing pod: %v, want nil", err)
	}
	if boxes, _ := r.List(ctx); len(boxes) != 0 {
		t.Errorf("after Destroy, List = %+v", boxes)
	}
}

func TestUnsupported(t *testing.T) {
	r := &Runtime{}
	if err := r.CheckSystemd(context.Background()); !errors.Is(err, runtime.ErrSystemdUnsupported) {
		t.Errorf("CheckSystemd = %v, want ErrSystemdUnsupported", err)
	}
	if err := r.Restart(context.Background(), "x"); !errors.Is(err, ErrRestartUnsupported) {
		t.Errorf("Restart = %v, want ErrRestartUnsupported", err)
	}
}
