# Documentation

`ntfsck` documentation follows the [Diátaxis](https://diataxis.fr/) split so you
can jump to the section that matches your intent.

| I want to… | Read |
| --- | --- |
| Get oriented and build the tool | [README](../README.md) |
| Understand how the code is structured | [Architecture](architecture.md) |
| Understand what happens on one run | [How it works](how-it-works.md) |
| Look up a flag, exit code, or JSON field | [CLI reference](reference/cli.md) |
| Look up a boot-sector, record, or attribute offset | [On-disk formats](reference/on-disk-formats.md) |
| Validate `ntfsck` against a real volume | [Validate a volume](how-to/validate-a-volume.md) |
| Add a new consistency check | [Add a check](how-to/add-a-check.md) |
| Read why a decision was made | [Architecture decisions](adr/README.md) |

## The four kinds

- **How-to guides** (`how-to/`) — task-oriented, ordered steps with a clear
  completion criterion.
- **Reference** (`reference/`) — exhaustive and terse: flags, exit codes, offsets,
  JSON fields.
- **Explanation** (`architecture.md`, `how-it-works.md`) — context and reasoning.
- **Tutorials / getting started** — the [README](../README.md) quick start.

## Conventions used across these docs

- All NTFS offsets are byte offsets from the start of the structure, and are
  written in hexadecimal.
- `$name` denotes an NTFS metadata file (`$mft`, `$mftmirr`, …).
- `internal/x` paths are Go package paths relative to the module root.
- Flowcharts are [Mermaid](https://mermaid.js.org/); GitHub renders them inline.
