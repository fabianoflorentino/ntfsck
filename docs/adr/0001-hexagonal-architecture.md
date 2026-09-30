# 0001 — Use a hexagonal architecture

- **Status:** Accepted
- **Date:** 2026-09-30

## Context

`ntfsck` has to read a real block device in production but must be tested without
one, and it must be able to grow from a read-only checker into a repairer without
a rewrite. The interesting logic — decoding on-disk structures, locating the
`$mft`, comparing records — is pure, but the thing it operates on is an unsafe,
stateful, I/O-bound medium.

The original project specification is written in Rust and emphasises SOLID. In Go
the idiomatic expression of those principles is small interfaces, dependency
inversion, and explicit wiring, not class hierarchies.

## Decision

Organise the code as ports and adapters:

- `internal/ntfs` is pure decoding over `[]byte` and performs no I/O.
- The core (`internal/volume`, `internal/check`, `internal/repair`,
  `internal/report`) depends only on narrow interfaces it declares itself.
- `internal/device` (real medium) and `internal/volumeimg` (in-memory fixtures)
  are interchangeable adapters behind the `volume.Source` port.
- Dependencies are wired by hand in `internal/cli.openVolume`; no DI framework.

Interfaces are declared at the consumer (`volume.Source`, `check.Volume`,
`repair.Volume`) rather than beside their implementations, so each consumer names
only the methods it calls.

## Consequences

Positive:

- The whole decoder and core are testable against byte slices, with no root and
  no device.
- A new medium is a new adapter; the core does not change.
- Read-only is expressed in types: `--check` depends on a read-only `Source`, and
  `repair.Volume` has no write method at all.

Negative:

- More packages and a hand-written wiring seam to maintain.
- Interface methods must be added deliberately when a new check needs a new
  capability, rather than reaching for a concrete type.

## Alternatives considered

- **A single package.** Simplest to start, but couples decoding to device I/O and
  makes tests need a real volume.
- **A `ports` package holding every interface.** Rejected: it recreates
  interface pollution and hides which consumer needs what.
- **A DI framework (wire, dig, fx).** Rejected for now: the graph is small and a
  hand-written constructor in `internal/cli.openVolume` is clearer. Revisit if the
  graph grows past a handful of components.
