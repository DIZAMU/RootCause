# ADR 0008: English and Norwegian from day one, helpdesk tickets

Status: Accepted (Phase 1). Recorded 2026-10-27.

## Context

The owner works in both languages, and troubleshooting at work starts with a ticket, not a task list.

## Decision

- Every user-facing string lives in `locales/{en,no}.json` or the exercise's `i18n/{en,no}.yaml`. CI fails if a key exists in one language and not the other.
- Exercises are framed as helpdesk tickets from a named sender. Feedback describes the symptom, never the fix.

## Consequences

- No user-facing text in Go or React.
- Every new exercise or screen costs two translations.
