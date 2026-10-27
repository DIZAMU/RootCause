package kubernetes

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"

	"github.com/DIZAMU/RootCause/internal/exercise"
	"github.com/DIZAMU/RootCause/internal/runtime"
	"github.com/DIZAMU/RootCause/internal/session"
)

// The identities that create pods in sandboxes: the API, and Argo CD's
// application controller, which can write anywhere in the cluster.
var admissionIdentities = map[string]string{
	"api":    "system:serviceaccount:rootcause-system:rootcause-api",
	"argocd": "system:serviceaccount:argocd:argocd-application-controller",
}

// The policy's messages (deploy/k8s/admission.yaml), by rule. A denial must
// carry the rule's own message, so a refusal by Pod Security Admission or
// RBAC never passes as the policy's.
var admissionMessages = map[string]string{
	"a": "a sandbox pod must set hostUsers to false",
	"b": "may not use the host network, PID or IPC namespace",
	"c": "may not mount a hostPath volume",
	"d": "no container in a sandbox pod may be privileged",
	"e": "must set allowPrivilegeEscalation to false",
	"f": "must drop ALL capabilities",
	"g": "may add back only CHOWN, DAC_OVERRIDE",
	"h": "RuntimeDefault seccomp profile",
	"i": "must set automountServiceAccountToken to false",
	"j": "must use imagePullPolicy Never",
	"k": "must be a rootcause/ exercise image",
	"l": "may use only emptyDir volumes",
}

// TestSandboxAdmission checks the admission policy on sandboxes against the
// pods the runtime really builds: every exercise's session pod (and runner
// pod, for runner checks) from session.SandboxSpec and podFor must be
// admitted, and each forbidden change to a real pod must be denied, for each
// identity in admissionIdentities. Each case names the layer that must refuse
// it, with that layer's message: the policy's rule, Pod Security Admission
// (which runs first), or API validation (which refuses host namespaces with
// hostUsers false before any admission). Pods are created
// with a server-side dry run while impersonating the identity, as kubectl
// --as does. One real pod is created, for the ephemeral container case, and
// deleted at the end.
//
// It needs a cluster with the policy and runs only with RC_CLUSTER_ADMISSION
// set; scripts/k8s-prove.sh sets it. RC_CLUSTER_CONTEXT names the kubeconfig
// context (default k3d-rootcause), whose user must be allowed to impersonate.
func TestSandboxAdmission(t *testing.T) {
	if os.Getenv("RC_CLUSTER_ADMISSION") == "" {
		t.Skip("RC_CLUSTER_ADMISSION is not set")
	}
	kubeContext := os.Getenv("RC_CLUSTER_CONTEXT")
	if kubeContext == "" {
		kubeContext = "k3d-rootcause"
	}
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{CurrentContext: kubeContext}).ClientConfig()
	if err != nil {
		t.Fatalf("kubeconfig context %s: %v", kubeContext, err)
	}
	ctx := context.Background()

	base := perm01Pod(t)
	allowed := realPods(t)

	for who, user := range admissionIdentities {
		cs := impersonating(t, config, user)
		t.Run(who, func(t *testing.T) {
			for _, p := range allowed {
				t.Run("allow/"+p.Name, func(t *testing.T) {
					if err := dryRunCreate(ctx, cs, p); err != nil {
						t.Fatalf("the runtime's own pod was refused: %v", err)
					}
				})
			}
			for _, c := range deniedCases() {
				t.Run("deny/"+c.rule+"/"+c.name+"/by-"+c.by, func(t *testing.T) {
					pod := base.DeepCopy()
					c.mutate(pod)
					expectRefused(t, dryRunCreate(ctx, cs, pod), c)
				})
			}
		})
	}

	// Ephemeral containers cannot be set when a pod is created, only added
	// to a running one. The API holds no right to add them at all; Argo CD
	// does, so the policy must refuse a non-compliant one.
	admin, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	target := base.DeepCopy()
	target.Name = "rootcause-admission-target"
	if _, err := admin.CoreV1().Pods(Namespace).Create(ctx, target, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create the ephemeral container target: %v", err)
	}
	t.Cleanup(func() {
		grace := int64(0)
		_ = admin.CoreV1().Pods(Namespace).Delete(context.Background(), target.Name, metav1.DeleteOptions{GracePeriodSeconds: &grace})
	})
	t.Run("argocd/deny/j/ephemeral-pull-IfNotPresent/by-policy", func(t *testing.T) {
		cs := impersonating(t, config, admissionIdentities["argocd"])
		expectRefused(t, addEphemeral(ctx, cs, target.Name), deniedCase{rule: "j", by: byPolicy})
	})
	t.Run("api/forbidden/ephemeral", func(t *testing.T) {
		cs := impersonating(t, config, admissionIdentities["api"])
		err := addEphemeral(ctx, cs, target.Name)
		if !apierrors.IsForbidden(err) || strings.Contains(err.Error(), "ValidatingAdmissionPolicy") {
			t.Fatalf("got %v, want an RBAC refusal before admission", err)
		}
	})
}

// Who must refuse a denied case.
const (
	byPolicy     = "policy"     // the admission policy, with the rule's message
	byPSA        = "psa"        // Pod Security Admission baseline, before the policy
	byValidation = "validation" // API validation, before any admission
)

// deniedCase is one forbidden change to a compliant pod. rule is the policy
// rule it breaks; by is the layer that must refuse it, and want, for a layer
// other than the policy, a part of that layer's message.
type deniedCase struct {
	rule, name, by, want string
	mutate               func(*corev1.Pod)
}

func deniedCases() []deniedCase {
	yes := true
	sc := func(p *corev1.Pod) *corev1.SecurityContext { return p.Spec.Containers[0].SecurityContext }
	privileged := func(c *corev1.Container) {
		c.SecurityContext = c.SecurityContext.DeepCopy()
		c.SecurityContext.Privileged = &yes
		c.SecurityContext.AllowPrivilegeEscalation = &yes // privileged implies it; validation refuses false
	}
	volume := func(name string, src corev1.VolumeSource) func(*corev1.Pod) {
		return func(p *corev1.Pod) {
			p.Spec.Volumes = append(p.Spec.Volumes, corev1.Volume{Name: name, VolumeSource: src})
		}
	}
	return []deniedCase{
		{rule: "a", name: "hostUsers-unset", by: byPolicy, mutate: func(p *corev1.Pod) { p.Spec.HostUsers = nil }},
		{rule: "a", name: "hostUsers-true", by: byPolicy, mutate: func(p *corev1.Pod) { p.Spec.HostUsers = &yes }},
		{rule: "b", name: "hostNetwork", by: byValidation, want: "when `hostUsers` is false",
			mutate: func(p *corev1.Pod) { p.Spec.HostNetwork = true }},
		{rule: "b", name: "hostPID", by: byValidation, want: "when `hostUsers` is false",
			mutate: func(p *corev1.Pod) { p.Spec.ShareProcessNamespace = nil; p.Spec.HostPID = true }},
		{rule: "b", name: "hostIPC", by: byValidation, want: "when `hostUsers` is false",
			mutate: func(p *corev1.Pod) { p.Spec.HostIPC = true }},
		{rule: "c", name: "hostPath", by: byPSA, want: "hostPath volumes",
			mutate: volume("host", corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/"}})},
		{rule: "d", name: "privileged", by: byPSA, want: "privileged",
			mutate: func(p *corev1.Pod) { privileged(&p.Spec.Containers[0]) }},
		{rule: "d", name: "privileged-init-container", by: byPSA, want: "privileged", mutate: func(p *corev1.Pod) {
			init := *p.Spec.Containers[0].DeepCopy()
			init.Name = "init"
			privileged(&init)
			p.Spec.InitContainers = append(p.Spec.InitContainers, init)
		}},
		{rule: "e", name: "allowPrivilegeEscalation-true", by: byPolicy,
			mutate: func(p *corev1.Pod) { sc(p).AllowPrivilegeEscalation = &yes }},
		{rule: "e", name: "allowPrivilegeEscalation-unset", by: byPolicy,
			mutate: func(p *corev1.Pod) { sc(p).AllowPrivilegeEscalation = nil }},
		{rule: "f", name: "no-drop-ALL", by: byPolicy, mutate: func(p *corev1.Pod) { sc(p).Capabilities.Drop = nil }},
		{rule: "g", name: "add-SYS_ADMIN", by: byPSA, want: "non-default capabilities",
			mutate: func(p *corev1.Pod) { sc(p).Capabilities.Add = append(sc(p).Capabilities.Add, "SYS_ADMIN") }},
		// MKNOD is in Pod Security's baseline but not in the exercise allowlist.
		{rule: "g", name: "add-MKNOD", by: byPolicy,
			mutate: func(p *corev1.Pod) { sc(p).Capabilities.Add = append(sc(p).Capabilities.Add, "MKNOD") }},
		{rule: "g", name: "add-MKNOD-init-container", by: byPolicy, mutate: func(p *corev1.Pod) {
			init := *p.Spec.Containers[0].DeepCopy()
			init.Name = "init"
			init.SecurityContext = init.SecurityContext.DeepCopy()
			init.SecurityContext.Capabilities.Add = append(init.SecurityContext.Capabilities.Add, "MKNOD")
			p.Spec.InitContainers = append(p.Spec.InitContainers, init)
		}},
		{rule: "h", name: "container-unconfined", by: byPSA, want: "seccompProfile", mutate: func(p *corev1.Pod) {
			sc(p).SeccompProfile = &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeUnconfined}
		}},
		// A Localhost profile passes Pod Security's baseline.
		{rule: "h", name: "container-localhost-profile", by: byPolicy, mutate: func(p *corev1.Pod) {
			profile := "profiles/any.json"
			sc(p).SeccompProfile = &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeLocalhost, LocalhostProfile: &profile}
		}},
		{rule: "h", name: "no-pod-seccomp", by: byPolicy, mutate: func(p *corev1.Pod) { p.Spec.SecurityContext.SeccompProfile = nil }},
		{rule: "i", name: "automount-unset", by: byPolicy, mutate: func(p *corev1.Pod) { p.Spec.AutomountServiceAccountToken = nil }},
		{rule: "i", name: "automount-true", by: byPolicy, mutate: func(p *corev1.Pod) { p.Spec.AutomountServiceAccountToken = &yes }},
		{rule: "j", name: "pull-IfNotPresent", by: byPolicy,
			mutate: func(p *corev1.Pod) { p.Spec.Containers[0].ImagePullPolicy = corev1.PullIfNotPresent }},
		{rule: "k", name: "foreign-image", by: byPolicy,
			mutate: func(p *corev1.Pod) { p.Spec.Containers[0].Image = "docker.io/library/busybox:1.37" }},
		{rule: "l", name: "secret-volume", by: byPolicy,
			mutate: volume("secret", corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "anything"}})},
		{rule: "l", name: "configmap-volume", by: byPolicy, mutate: volume("config", corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "anything"}}})},
		{rule: "l", name: "projected-volume", by: byPolicy,
			mutate: volume("projected", corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{}})},
		{rule: "l", name: "pvc-volume", by: byPolicy, mutate: volume("pvc", corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "anything"}})},
	}
}

// realPods builds the pod of every exercise's sandbox, and of its runner for
// runner checks, the way a session creates them. init: systemd exercises are
// refused on Kubernetes before a pod is built, so they are left out.
func realPods(t *testing.T) []*corev1.Pod {
	t.Helper()
	dirs, err := filepath.Glob("../../../exercises/*/exercise.yaml")
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no exercises found: %v", err)
	}
	var pods []*corev1.Pod
	for _, f := range dirs {
		ex, err := exercise.Load(filepath.Dir(f))
		if err != nil {
			t.Fatal(err)
		}
		if ex.Init == "systemd" {
			continue
		}
		spec := session.SandboxSpec(ex, "s-0000000000000000", "rootcause_admission_test")
		specs := []runtime.SandboxSpec{spec}
		if ex.Check.RunsFrom == "runner" {
			specs = append(specs, session.RunnerSpec(spec))
		}
		for _, s := range specs {
			pod, err := podFor(s, Namespace, nil)
			if err != nil {
				t.Fatal(err)
			}
			pod.Name = strings.Replace(pod.Name, "rootcause-s-0000000000000000", "rootcause-admission-"+ex.ID, 1)
			pods = append(pods, pod)
		}
	}
	return pods
}

// perm01Pod is the real session pod of perm-01, the base every denied case
// changes one thing in.
func perm01Pod(t *testing.T) *corev1.Pod {
	t.Helper()
	ex, err := exercise.Load("../../../exercises/perm-01-shared-folder")
	if err != nil {
		t.Fatal(err)
	}
	pod, err := podFor(session.SandboxSpec(ex, "s-0000000000000000", "rootcause_admission_test"), Namespace, nil)
	if err != nil {
		t.Fatal(err)
	}
	pod.Name = "rootcause-admission-base"
	return pod
}

func impersonating(t *testing.T, config *rest.Config, user string) kubernetes.Interface {
	t.Helper()
	c := rest.CopyConfig(config)
	c.Impersonate = rest.ImpersonationConfig{UserName: user}
	cs, err := kubernetes.NewForConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	return cs
}

func dryRunCreate(ctx context.Context, cs kubernetes.Interface, pod *corev1.Pod) error {
	_, err := cs.CoreV1().Pods(Namespace).Create(ctx, pod, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}})
	return err
}

// addEphemeral asks, as a dry run, to add an ephemeral container to the pod
// that is compliant except for its pull policy, which only the policy checks.
// The pod may change while it starts, so a conflict is retried.
func addEphemeral(ctx context.Context, cs kubernetes.Interface, name string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		pod, err := cs.CoreV1().Pods(Namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		c := pod.Spec.Containers[0]
		pod.Spec.EphemeralContainers = append(pod.Spec.EphemeralContainers, corev1.EphemeralContainer{
			EphemeralContainerCommon: corev1.EphemeralContainerCommon{
				Name: "debug", Image: c.Image, ImagePullPolicy: corev1.PullIfNotPresent, Command: []string{"sleep", "1"},
				SecurityContext: c.SecurityContext.DeepCopy(),
			},
		})
		_, err = cs.CoreV1().Pods(Namespace).UpdateEphemeralContainers(ctx, name, pod, metav1.UpdateOptions{DryRun: []string{metav1.DryRunAll}})
		return err
	})
}

// expectRefused fails unless err is the refusal the case names: the policy's,
// with the rule's message, or Pod Security's or API validation's, with the
// case's want. Impersonation the kubeconfig user may not do fails with a hint.
func expectRefused(t *testing.T, err error, c deniedCase) {
	t.Helper()
	if err == nil {
		t.Fatalf("admitted; it breaks rule %s", c.rule)
	}
	msg := err.Error()
	fromPolicy := strings.Contains(msg, "ValidatingAdmissionPolicy 'rootcause-sandbox-pods'")
	switch {
	case strings.Contains(msg, "cannot impersonate"):
		t.Fatalf("the kubeconfig user may not impersonate (k3d's admin can): %v", err)
	case c.by == byPolicy && !fromPolicy:
		t.Fatalf("refused, but not by the policy: %v", err)
	case c.by == byPolicy && !strings.Contains(msg, admissionMessages[c.rule]):
		t.Fatalf("refused by the policy, but not with rule %s's message %q: %v", c.rule, admissionMessages[c.rule], err)
	case c.by == byPSA && !strings.Contains(msg, "violates PodSecurity"):
		t.Fatalf("want a Pod Security refusal: %v", err)
	case c.by == byValidation && !apierrors.IsInvalid(err):
		t.Fatalf("want an API validation refusal: %v", err)
	case c.by != byPolicy && !strings.Contains(msg, c.want):
		t.Fatalf("refused by %s, but without %q: %v", c.by, c.want, err)
	}
}
