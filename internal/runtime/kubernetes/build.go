package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Cluster is the name of the k3d cluster in deploy/k8s/k3d.yaml.
const Cluster = "rootcause"

// Importer builds an exercise image with docker on the host and imports it
// into every node of the k3d cluster, where sandbox pods use it without a
// pull. It is the builder for the CLI outside the cluster.
type Importer struct {
	Cluster string
}

// BuildImage builds the image, then runs k3d image import.
func (i Importer) BuildImage(ctx context.Context, tag, dir string) error {
	if err := run(ctx, "docker", "build", "-t", tag, dir); err != nil {
		return err
	}
	return run(ctx, "k3d", "image", "import", "--cluster", i.Cluster, tag)
}

// Preloaded is the builder inside the cluster, which cannot build images:
// scripts/k8s-up.sh imports every exercise image before the API starts. A
// missing image shows up as the pod's ErrImageNeverPull in Diagnose.
type Preloaded struct{}

// BuildImage does nothing.
func (Preloaded) BuildImage(context.Context, string, string) error { return nil }

// run runs a host command and returns its stderr in the error.
func run(ctx context.Context, name string, args ...string) error {
	c := exec.CommandContext(ctx, name, args...)
	var errBuf bytes.Buffer
	c.Stderr = &errBuf
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(errBuf.String()))
	}
	return nil
}
