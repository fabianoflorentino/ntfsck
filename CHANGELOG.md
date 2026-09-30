# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

<!--
Version links are added when the first release is tagged, for example:

[unreleased]: https://github.com/fabianoflorentino/ntfsck/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/fabianoflorentino/ntfsck/releases/tag/v0.1.0
-->

## [Unreleased]

### Added

- Hexagonal core: `internal/ntfs` decodes the boot sector, MFT record headers,
  attributes, and data runlists without performing I/O.
- `internal/volume` opens a source, resolves the `$mft` extent from record 0,
  and maps MFT records to byte offsets, including fragmented extents.
- `internal/check` compares the leading `$mftmirr` records against `$mft` and
  reports per-record and aggregate findings.
- `internal/repair` turns findings into an ordered, deduplicated plan without
  writing to the volume.
- `internal/report` renders reports as text or JSON.
- `internal/device` adapts a file or block device, enforcing read-only handles.
- `internal/volumeimg` builds synthetic NTFS volumes for tests.
- `cmd/ntfsck` and `internal/cli` provide the command tree, flags, and exit
  codes `0` OK, `1` failure, `2` usage, `3` inconsistent.
