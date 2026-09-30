# CLI reference

## Synopsis

```
ntfsck [flags] <device>
```

- `<device>` — a block device (for example `/dev/sdb1`) or a filesystem image
  file. Exactly one is accepted (`cobra.MaximumNArgs(1)`); omitting it is a usage
  error.
- Running with no action flag prints nothing and exits `0`. Pass `--check`,
  `--repair`, or `--dump-mft` to do something.

`ntfsck` reads no configuration file and no environment variables. Everything is
a flag.

## Flags

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--check` | bool | `false` | Analyze the volume without writing anything. |
| `--repair` | bool | `false` | Plan the repairs the volume needs. |
| `--dry-run` | bool | `false` | With `--repair`, plan only and never open the volume for writing. |
| `--dump-mft` | bool | `false` | Print one MFT record header. |
| `--mft-record` | int | `0` | MFT record index to print with `--dump-mft`. |
| `--output` | string | `text` | Report format: `text` or `json`. |
| `-h`, `--help` | bool | | Print help and exit. |
| `-v`, `--version` | bool | | Print the version and exit. |

### Mutual exclusivity

- `--check`, `--repair`, and `--dump-mft` are mutually exclusive.
- `--dry-run` is mutually exclusive with `--check` and `--dump-mft` (it only
  makes sense with `--repair`).

Combining them is a usage error (exit `2`). A scan that also writes is the
ambiguity `ntfsck` exists to remove.

## Exit codes

| Code | Constant | Meaning |
| --- | --- | --- |
| `0` | `ExitOK` | The volume is consistent, or the request succeeded. |
| `1` | `ExitFailure` | `ntfsck` could not complete the request (I/O error, unreadable structure, unimplemented repair). |
| `2` | `ExitUsage` | The command line was wrong. |
| `3` | `ExitInconsistent` | The check completed and found problems. |

The split between `1` and `3` lets a script distinguish "the volume is broken"
from "the tool could not run".

## Output formats

### `text`

An operator-facing summary: volume line, geometry, one line per finding, the list
of structures not checked, and a verdict.

```console
$ ntfsck --check /dev/sdb1
NTFS volume:  NTFS
Cluster size: 4096 bytes
MFT records: 28

[OK]    $mftmirr  record 0 matches $mft
[OK]    $mftmirr  record 1 matches $mft
[OK]    $mftmirr  record 2 matches $mft
[WARN]  $mftmirr  record 3 ($volume) differs from $mft
[WARN]  $mftmirr  1 of 4 mirrored records diverge from $mft

Not checked yet: $bitmap, $logfile, $secure

Filesystem is inconsistent. Repair recommended.
```

Severity tags are `OK`, `WARN`, `ERROR`, and `UNKNOWN`. Only `WARN` and above
make a report inconsistent.

### `json`

A stable document for scripts. Every field carries an explicit tag, so a Go
rename cannot change the wire format.

```json
{
  "volume": "NTFS    ",
  "cluster_size": 4096,
  "record_count": 28,
  "consistent": false,
  "findings": [
    { "structure": "$mftmirr", "severity": "OK", "detail": "record 0 matches $mft" },
    { "structure": "$mftmirr", "severity": "WARN", "detail": "record 3 ($volume) differs from $mft" }
  ],
  "not_covered": ["$bitmap", "$logfile", "$secure"]
}
```

| Field | Type | Meaning |
| --- | --- | --- |
| `volume` | string | The volume's OEM signature (`NTFS    `). |
| `cluster_size` | int | Cluster size in bytes. |
| `record_count` | int | Number of MFT records in the `$mft` extent. |
| `consistent` | bool | `false` when any finding reached `WARN` or above. |
| `findings` | array | One object per structure examined. |
| `findings[].structure` | string | The NTFS structure, e.g. `$mftmirr`. |
| `findings[].severity` | string | `OK`, `WARN`, `ERROR`, or `UNKNOWN`. |
| `findings[].detail` | string | One sentence explaining the verdict. |
| `not_covered` | array | Structures this build does not examine yet. |

## `--dump-mft`

Prints the decoded `FILE` header of one MFT record, without running a check.

```console
$ ntfsck --dump-mft --mft-record 0 /dev/sdb1
record 0  $mft
  magic:        FILE
  sequence:     1
  in use:       true
  directory:    false
  hard links:   1
  attrs at:     56
  bytes in use: 408
```

The field name is the fixed NTFS system name for records 0–11 (`$mft`,
`$mftmirr`, `$logfile`, `$volume`, `$attrdef`, `.`, `$bitmap`, `$boot`,
`$badclus`, `$secure`, `$upcase`, `$extend`), or `(user file)` otherwise.

## `--repair`

Runs a check, turns the findings into a plan, and prints it. Nothing is written;
the trailing line says so.

```console
$ ntfsck --repair --dry-run /dev/sdb1
$mftmirr: rewrite the mirrored copy at cluster 8191 from $mft

1 action(s) planned; writing is not enabled yet.
```

- A consistent volume prints the normal report and exits `0`.
- A plan with actions exits `3`.
- A structure that has no modelled fix exits `1` with `ErrNotImplemented` rather
  than emitting a partial plan.
