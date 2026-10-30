package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/DIZAMU/RootCause/internal/runtime"
	"github.com/DIZAMU/RootCause/internal/runtime/docker"
	"github.com/DIZAMU/RootCause/internal/runtime/kubernetes"
	"github.com/DIZAMU/RootCause/internal/session"
)

// EnvRuntime selects the sandbox runtime: "docker" (the default, v1) or
// "kubernetes" (v2, ARCHITECTURE.md section 8.2).
const EnvRuntime = "RC_RUNTIME"

// Backend is the runtime the engine drives, with the builder for its images
// and, where the runtime has one, its interactive shell.
type Backend struct {
	Runtime runtime.Runtime
	Builder session.Builder
	Sheller runtime.Sheller // nil when the runtime has no terminal shell
}

// RuntimeName returns the runtime RC_RUNTIME selects, or an error for an
// unknown value.
func RuntimeName() (string, error) {
	switch v := os.Getenv(EnvRuntime); v {
	case "", "docker":
		return "docker", nil
	case "kubernetes":
		return "kubernetes", nil
	default:
		return "", fmt.Errorf("%s=%q: want docker or kubernetes", EnvRuntime, v)
	}
}

// NewBackend builds the backend RC_RUNTIME selects. The docker runtime must
// keep sandbox root apart from host root (ARCHITECTURE.md section 13.1); the
// kubernetes runtime gives every sandbox pod its own user namespace itself.
// Errors are already the translatable user errors.
func NewBackend(ctx context.Context) (Backend, error) {
	name, err := RuntimeName()
	if err != nil {
		return Backend{}, err
	}
	if name == "kubernetes" {
		rt, err := kubernetes.New()
		if err != nil {
			return Backend{}, fail("error.no_kubernetes", err)
		}
		var b session.Builder = kubernetes.Importer{Cluster: kubernetes.Cluster}
		if rt.InCluster() {
			b = kubernetes.Preloaded{}
		}
		return Backend{Runtime: rt, Builder: b}, nil
	}

	rt, err := docker.New()
	if err != nil {
		return Backend{}, fail("error.no_docker", err)
	}
	if err := rt.CheckIsolation(ctx); err != nil {
		if errors.Is(err, docker.ErrNotIsolated) {
			return Backend{}, fail("error.not_isolated", nil)
		}
		return Backend{}, err
	}
	return Backend{Runtime: rt, Builder: rt, Sheller: rt}, nil
}
