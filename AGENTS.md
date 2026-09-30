# AGENTS.md

Before any Go coding, review, debugging, troubleshooting, or setup task, load the `fabianoflorentino/golang-agent-skills@golang-how-to` skill first — it routes to whichever other Go skills the task needs.

## Project

ntfsck is a read-only-first NTFS consistency checker and repairer for Linux, a Go reimplementation of the Rust project specification in `ntfsck_projeto_reparador_ntfs_linux.pdf`.

- Module: `github.com/fabianoflorentino/ntfsck`
- Architecture: hexagonal. `internal/volume` is the core; `internal/ntfs` is pure format decoding; `internal/device` and `internal/volumeimg` are the two adapters behind the `volume.Source` port. Dependencies are wired manually in `internal/cli.openVolume`; there is no DI framework.
- Interfaces are declared where they are consumed (`volume.Source`, `check.Volume`, `repair.Volume`), not alongside their implementations.

## Package map

- `cmd/ntfsck` — `main`, plus `Run(args, stdout, stderr) int` for testability.
- `internal/cli` — Cobra command tree and exit codes.
- `internal/ntfs` — boot sector, MFT record header, attribute and runlist decoding.
- `internal/volume` — opens a source, resolves the `$mft` extent from record 0, maps records to offsets.
- `internal/check` — consistency findings.
- `internal/repair` — repair plans (no writes yet).
- `internal/report` — text and JSON rendering.
- `internal/device` — real file/block-device adapter.
- `internal/volumeimg` — in-memory fixture adapter for tests.

## Conventions

- All on-disk multi-byte values are little-endian; every offset used to decode a structure states the field it reads.
- Errors are lowercase, without trailing punctuation, wrapped with `%w`; sentinel errors are exported (`ErrNoRuns`, `ErrRecordOutOfRange`, ...).
- `NotCovered` in a report is kept separate from findings so an unimplemented check cannot turn a healthy volume into an inconsistent one.
- Exit codes: `0` OK, `1` failure, `2` usage, `3` inconsistent volume.

## Commands

- Build: `make build` (writes `bin/ntfsck`).
- Test: `go test ./...`; with the race detector: `go test -race ./...`.
- Lint: `golangci-lint run ./...`.
- Validate against a real volume: `mkntfs -F -f -L testvol /tmp/opencode/real.img && bin/ntfsck --check /tmp/opencode/real.img`.
