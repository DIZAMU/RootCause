# Security

RootCause gives the learner root inside a sandbox on purpose, so the sandbox isolation matters. The design and its known limits are described in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) (section 13) and in [ADR 0019](docs/adr/0019-known-limits.md).

## Reporting a vulnerability

Please report a vulnerability privately through GitHub. Open the **Security** tab of this repository and choose **Report a vulnerability**. Do not open a public issue for it.

Helpful things to include are what you found, how to reproduce it, and which mode it affects (Docker mode or Kubernetes mode). I will reply as soon as I can.

## In scope

- A way out of a sandbox, to the host or to another sandbox
- A way for a sandbox to reach the internet, `rootcause-system` or the Kubernetes API
- A way for another website to call the local API or read its token
- A secret that ends up in the repository, in a log or on a command line

## Out of scope

The limits already listed in ADR 0019, such as a learner faking or reading a check from inside their own sandbox, filling their own disk from a sandbox, or k3d's privileged nodes.
