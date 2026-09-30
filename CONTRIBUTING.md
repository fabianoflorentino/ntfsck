# Contributing to ntfsck

Thanks for your interest in improving `ntfsck`.

## Prerequisites

- Linux (the checker reads raw devices and files)
- Go 1.27 or later
- `make` (optional, but it wraps every task below)
- [`golangci-lint`](https://golangci-lint.run/) v2 for linting
- `mkntfs` (from `ntfs-3g`) for the real-volume validation flow

## Quick Start

```bash
git clone https://github.com/fabianoflorentino/ntfsck.git
cd ntfsck

make build        # build bin/ntfsck
make test         # go test ./...
make test-race    # go test -race ./...
make lint         # golangci-lint run ./...
make help         # list every target
```

The bar: clone, change, and run the tests in under ten minutes.

## Development Workflow

1. Fork the repository.
2. Create a branch: `git checkout -b feat/my-feature`.
3. Make the change and add tests for new behavior.
4. Run `make test`, `make test-race`, and `make lint`.
5. Commit with a descriptive message.
6. Push and open a pull request.

## Code Guidelines

- Follow [Effective Go](https://go.dev/doc/effective_go) and the existing style:
  comments explain *why* a structure is decoded the way it is, not what the code
  already says.
- Add doc comments to all exported symbols; the first sentence is the `go doc`
  summary and names the declared item.
- Write table-driven tests. Fixtures are built in memory by `internal/volumeimg`
  so tests never need root or a real device.
- Keep test coverage on the path you touched; `make cover` reports per-function
  coverage.

### On-disk decoding rules

These keep the decoder honest and reviewable:

- All multi-byte values are little-endian.
- Every offset used to decode a structure states the field it reads.
- Every `int8`/`int16`/`int32` reinterpretation of a field is deliberate and
  reflects the format's signed encoding (runlist deltas, the boot sector's
  clusters-per-record field).
- Decoding performs no I/O: `internal/ntfs` takes a `[]byte` and returns a
  value, which is what makes it testable against a slice.

### Architecture rules

- Interfaces are declared where they are consumed (`volume.Source`,
  `check.Volume`, `repair.Volume`), not alongside their implementations.
- New adapters implement an existing port; the core must not import an adapter.
- Dependencies are wired explicitly in `internal/cli.openVolume`; there is no DI
  framework. Keep it that way until the graph justifies one.

### Adding a check

See [docs/how-to/add-a-check.md](docs/how-to/add-a-check.md) for the step-by-step
recipe and the fixture it expects.

## Reporting Issues

Use [GitHub Issues](https://github.com/fabianoflorentino/ntfsck/issues). Include:

- `ntfsck --version` output and `go version`
- Operating system and architecture
- How the volume was created (`mkntfs` flags, size)
- Steps to reproduce
- Expected vs actual behavior, with the full report output
