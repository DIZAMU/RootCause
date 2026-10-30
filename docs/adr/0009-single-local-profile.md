# ADR 0009 · No accounts, one local profile

Accepted on 2026-10-27.

## Context

RootCause is a single-user practice tool that runs on the learner's own machine.

## Decision

- There are no accounts and no login. One learner row is named after the OS user.

## Consequences

- There is no authentication code to secure. The dashboard's protections are about other websites in the same browser, not about other users.
- Supporting several users would need a new design.
