# How to add a consistency check

**Goal:** teach `ntfsck` to judge one more NTFS structure, with a fixture and
tests, without touching the CLI or the device layer.

**Done when:** the new structure has a `check*` method producing `[]Finding`, a
table-driven test over `internal/volumeimg` fixtures, and the structure no longer
appears in `Report.NotCovered`. `make test test-race lint` is green.

## Steps

### 1. Pick the structure and its severity

Decide what a violation means for the operator:

- `SeverityWarn` — the volume still mounts; the operator should know.
- `SeverityError` — the volume is unusable.
- `SeverityOK` — the structure verified clean.

Structures you cannot judge must stay in `Report.NotCovered`; never emit `OK` for
something you did not actually verify.

### 2. Add a method on `check.Checker`

Follow `checkMirror` in `internal/check/check.go`. Keep the signature
`func (c *Checker) checkX(ctx context.Context) ([]Finding, error)`, check
`ctx.Err()` between records, and return findings in a stable order.

```go
func (c *Checker) checkBitmap(ctx context.Context) ([]Finding, error) {
	// Read through the narrow port; never reach for a concrete type.
	raw, err := c.volume.MFTRecordBytes(ntfs.Bitmap)
	if err != nil {
		return nil, fmt.Errorf("check: reading $bitmap: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("check: %w", err)
	}

	// ... compare the bitmap against the cluster allocation the mft implies ...

	return findings, nil
}
```

A structure that cannot be read at all is itself a finding, not a command
failure: return an `ERROR` finding with `nil` error. A reader error that means
"`ntfsck` cannot run" stays an `error`.

### 3. Call it from `Run` and update `NotCovered`

In `Checker.Run`, append the method's findings and **remove** the structure from
the `NotCovered` slice.

### 4. Extend the port only if you must

`check.Volume` is deliberately narrow (`Boot`, `RecordCount`, `MFTRecord`,
`MFTRecordBytes`, `ReadAt`). If a check needs a new capability, add the smallest
method that serves it to the interface and to `*volume.Volume` — do not widen the
port with a general-purpose reader.

### 5. Provide a fixture

`internal/volumeimg` builds synthetic volumes. Add a constructor that produces
the fault you want to catch, mirroring `NewDiverged`, and keep the geometry
constants in one place so offsets stay explicit:

```go
func NewBadBitmap() *Bytes {
	img := New()
	// Corrupt the bitmap at a known cluster; state the field you write.
	// ...
	return img
}
```

### 6. Write a table-driven test

Tests live beside the package, use `t.Parallel()`, and assert on findings rather
than free-form text where possible. `TestCheckerRun` in
`internal/check/check_test.go` is the template:

```go
func TestCheckerRun_bitmap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		img  *volumeimg.Bytes
		want check.Severity
	}{
		{"clean bitmap", volumeimg.New(), check.SeverityOK},
		{"corrupt bitmap", volumeimg.NewBadBitmap(), check.SeverityWarn},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// open tt.img and assert the finding severity
		})
	}
}
```

### 7. Decide the repair

In `internal/repair`, either model the fix as an `Action`, or let the existing
`default` branch return `ErrNotImplemented`. Returning `ErrNotImplemented` is the
correct choice until the fix is fully modelled: a partial plan looks complete,
which is the failure mode this tool exists to remove. See
[ADR 0002](../adr/0002-read-only-first.md).

### 8. Run the gates

```bash
make test
make test-race
make lint
```

### 9. Document it

- Add the structure to the check table in the [README](../../README.md).
- Update the [CHANGELOG](../../CHANGELOG.md) under `Unreleased`.
- If it introduces offsets, add them to
  [On-disk formats](../reference/on-disk-formats.md).

## Checklist

- [ ] `check*` method returns `[]Finding`, checks `ctx`, no I/O of its own
- [ ] Structure removed from `Report.NotCovered`
- [ ] Fixture in `internal/volumeimg`
- [ ] Table-driven test covering clean and faulty cases
- [ ] Repair modelled, or `ErrNotImplemented` left in place
- [ ] `make test test-race lint` green
- [ ] README, CHANGELOG, and reference docs updated
