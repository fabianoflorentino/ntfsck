# 0002 — Be read-only first; plan before writing

- **Status:** Accepted
- **Date:** 2026-09-30

## Context

The tool exists because a real volume came back with `$MFTMirr != $MFT`, and
`ntfsfix` repaired it only partially. Repairing NTFS metadata is dangerous: a
wrong write to `$mftmirr` or `$bitmap` can turn a mountable volume into an
unreadable one. Operators point this tool at real, sometimes irreplaceable,
disks.

The project specification asks for `--check`, `--repair --dry-run`, and
`--repair` as distinct operations. That ordering is the insight: inspection and
planning are safe and should always come first.

## Decision

The build is read-only first.

- `--check` and `--repair --dry-run` open the source read-only. The device
  adapter fails every write on a read-only handle with `ErrReadOnly` before
  touching the medium.
- `--repair` produces an explicit, ordered `Plan` and prints it. **The current
  build does not apply it.**
- A finding with no modelled fix returns `repair.ErrNotImplemented`; the tool
  refuses to emit a partial plan.
- The write capability is deliberately absent from `repair.Volume` and from the
  read-only `Source` handle. Applying a plan will require a separate, narrower
  writable port introduced when every fix is modelled and tested.

## Consequences

Positive:

- An interrupted run can never leave a half-written structure, because nothing
  is written until a plan is complete and reviewed.
- The safest operations need no special privilege beyond read access.
- "Read-only" is enforced by the type system and the adapter, not by convention.

Negative:

- The tool is not yet a repairer; it diagnoses and plans. This is a deliberate
  limitation, tracked as the next phase.
- Every fix must be modelled before it ships, which is slower than writing first
  and testing later.

## Alternatives considered

- **Write directly on `--repair`.** Rejected: it makes the dangerous path the
  default and removes the operator's chance to review.
- **Expose a general writable `Source` to the core.** Rejected: it destroys the
  property that the checker physically cannot write. The write port will be
  narrower than the read port.
- **Silently skip structures with no fix and report partial success.** Rejected:
  a partial plan that looks complete is exactly the failure mode that motivated
  the project.
