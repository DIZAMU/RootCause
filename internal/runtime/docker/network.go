package docker

import (
	"context"
	"fmt"
	"strings"
)

// internalNetwork is the name of the docker network used for exercises whose
// network mode is "internal": a bridge network created with --internal, so
// sandboxes on it can reach each other but have no route off the host.
const internalNetwork = "rootcause-internal"

// EnsureInternalNetwork creates the internal network if it does not exist. It
// is idempotent. Callers use it before creating a sandbox with Network
// "internal".
func (r *Runtime) EnsureInternalNetwork(ctx context.Context) error {
	_, _, err := r.run(ctx, nil, "network", "inspect", internalNetwork)
	if err == nil {
		return nil // already exists
	}
	_, errOut, err := r.run(ctx, nil, "network", "create", "--internal", internalNetwork)
	if err != nil {
		// A concurrent create is not a failure.
		if strings.Contains(errOut, "already exists") {
			return nil
		}
		return fmt.Errorf("create internal network: %w: %s", err, strings.TrimSpace(errOut))
	}
	return nil
}
