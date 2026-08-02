# ADR 0001: Keep a Full Educational Project Skeleton

- Status: Accepted
- Date: 2026-07-25
- Review date: 2026-10-25

## Context

The architecture guide recommends creating only packages required by real behavior because
unused placeholders usually add noise. This repository is currently being used to teach the
complete project layout, and its initial requirement explicitly asks for every canonical
folder plus guidance about what belongs there.

## Decision

Create the complete canonical tree with compile-safe package declarations and explanatory
comments. Do not add framework or infrastructure dependencies until a feature requires
them. Keep only `main` and the no-op command entry point executable so the scaffold can be
validated with standard Go tooling.

## Alternatives

Create packages incrementally with the first vertical feature. That is the preferred
production approach, but it does not satisfy the educational requirement to inspect the
whole layout before implementation.

## Impact

The repository temporarily contains packages with no runtime behavior. When implementation
begins, each placeholder must either gain an owned responsibility or be removed. The review
date is a prompt to delete unused placeholders once the team understands the structure.
