#!/usr/bin/env bash
# Builds the marts in the cluster now, instead of waiting for the dbt
# CronJob's next run (deploy/k8s/dbt.yaml). Exit 0 if the build succeeded;
# otherwise the end of the dbt log is printed and the exit is 1.
set -euo pipefail

kc() { kubectl --context k3d-rootcause -n rootcause-system "$@"; }
job="dbt-now-$(date +%s)"
kc create job "$job" --from=cronjob/dbt >/dev/null
echo "started job $job"
# The Job ends Complete or Failed; wait for whichever comes first.
if kc wait "job/$job" --for=condition=Complete --timeout=300s >/dev/null 2>&1; then
	kc logs "job/$job" -c dbt | grep -E 'Done\.' || true
	echo "dbt build succeeded"
	exit 0
fi
kc logs "job/$job" -c dbt --tail=30 || true
echo "dbt build failed or did not finish within 300s (job $job)" >&2
exit 1
