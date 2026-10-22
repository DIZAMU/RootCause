# Healthy machine for perm-01: a plain Debian file server. The department
# group, its users and the share are created by setup.sh at session start, so
# the image holds no exercise state.
#
# Pinned by digest, so a rebuild always gets the same base. To update:
# docker pull debian:13-slim, then
# docker inspect --format '{{index .RepoDigests 0}}' debian:13-slim.
FROM debian:13-slim@sha256:62bd423aeee912639ca8ee681c2b762cb3e25137e0a92b20be455898a9f034ad

# procps: ps and friends. nano, vim-tiny: the learner's editors.
RUN apt-get update \
 && apt-get install -y --no-install-recommends procps nano vim-tiny \
 && rm -rf /var/lib/apt/lists/*

ENTRYPOINT ["sleep", "infinity"]
