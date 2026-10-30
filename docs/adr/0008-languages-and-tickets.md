# ADR 0008 · English and Norwegian from the start, framed as helpdesk tickets

Accepted on 2026-10-27.

## Context

The author works in both languages, and troubleshooting at work starts with a ticket, not a task list.

## Decision

- Every string a user sees lives in `locales/{en,no}.json` or in the exercise's `i18n/{en,no}.yaml`. CI fails if a key exists in one language and not the other.
- Exercises are framed as helpdesk tickets from a named sender. Feedback describes the symptom, never the fix.

## Consequences

- There is no user-facing text in the Go or React code.
- Every new exercise or screen needs two translations.
