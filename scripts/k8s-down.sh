#!/usr/bin/env bash
# Stops the k3d cluster to free memory. Its data (Postgres included) stays;
# scripts/k8s-up.sh starts it again. Deleting the cluster and its data is
# `k3d cluster delete rootcause`, by hand.
set -euo pipefail

k3d cluster stop rootcause
