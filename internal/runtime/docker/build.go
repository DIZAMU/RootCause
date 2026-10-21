package docker

import (
	"context"
	"fmt"
	"strings"
)

// BuildImage builds an exercise image from a context directory. Docker caches
// layers, so repeated builds of an unchanged exercise are fast. This is a v1
// concern: in v2 images are built in CI and pulled.
func (r *Runtime) BuildImage(ctx context.Context, tag, dir string) error {
	_, errOut, err := r.run(ctx, nil, "build", "-t", tag, dir)
	if err != nil {
		return fmt.Errorf("docker build %s: %w: %s", tag, err, strings.TrimSpace(errOut))
	}
	return nil
}
