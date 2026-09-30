# On-disk formats

This is the reference for the structures `internal/ntfs` decodes. Every offset is
a byte offset from the start of the structure named in the heading, written in
hexadecimal. **All multi-byte integers are little-endian.**

The decoder performs no I/O: each function takes a `[]byte` and returns a value,
which is what lets every structure be tested against a slice.

## Boot sector

A fixed 512-byte structure (`ntfs.SectorSize`) at offset 0 of the volume.
Decoded by `ntfs.ParseBootSector`.

| Offset | Size | Field | Notes |
| --- | --- | --- | --- |
| `0x03` | 8 | OEM signature | Always `NTFS    ` (`ntfs.OemSignature`). Mismatch → `ntfs.ErrNotNTFS`. |
| `0x0B` | 2 | Bytes per sector | |
| `0x0D` | 1 | Sectors per cluster | |
| `0x28` | 8 | Total sectors | Zero when unknown. |
| `0x30` | 8 | `$mft` zone cluster | Where record 0 lives. Not the runlist. |
| `0x38` | 8 | `$mftmirr` cluster | Where record 0's mirrored copy lives. |
| `0x40` | 1 signed | Clusters per MFT record | See encoding below. |
| `0x44` | 1 signed | Clusters per index record | Same encoding. |

Derived values:

- `ClusterSize() = BytesPerSector × SectorsPerCluster`
- `MFTRecordSize()` and `IndexRecordSize()` decode the signed field below.

### Clusters-per-record encoding

The field at `0x40`/`0x44` is signed:

| Value | Record size |
| --- | --- |
| `n ≥ 0` | `n × ClusterSize` |
| `n < 0` | `2^(-n)` bytes |

`0xF6` is `-10`, so the record size is `2^10 = 1024` bytes — the value every
volume created since NTFS 3.1 uses (`ntfs.RecordSize`). A magnitude above 20 is
rejected as nonsense.

> The boot sector so strongly suggests the `$mft` lives exactly at `0x30` that it
> is tempting to read `$mft` from there directly. Do not: the field is the
> *zone* cluster, and the authoritative runlist is inside record 0. See
> [How it works](../how-it-works.md#3-the-mft-extent-comes-from-record-0-not-the-boot-sector).

## MFT record header

The `FILE` header at the start of every MFT record. Decoded by
`ntfs.DecodeRecordHeader`.

| Offset | Size | Field | Notes |
| --- | --- | --- | --- |
| `0x00` | 4 | Magic | Always `FILE` (`ntfs.RecordMagic`). Mismatch → `ntfs.ErrBadRecordMagic`. |
| `0x04` | 2 | Fixup offset | Offset of the update sequence array. |
| `0x06` | 2 | Fixup count | Words covered by the update sequence. |
| `0x10` | 2 | Sequence number | Increments each time the record is reused. |
| `0x12` | 2 | Hard link count | Directory entries pointing at the record. |
| `0x14` | 2 | Attributes offset | Where the attribute list starts. |
| `0x16` | 2 | Flags | See below. |
| `0x18` | 4 | Bytes in use | Must not exceed the record size. |
| `0x1C` | 4 | Bytes allocated | The record size on disk. |
| `0x20` | 8 | Base record | Record this one extends; zero when it does not. |

The 8-byte LSN at `0x08` and the update sequence itself are not decoded. The
header is `0x20` bytes; `AttributesOffset` must be at least that and no larger
than the record.

### Record flags

| Bit | Constant | Meaning |
| --- | --- | --- |
| `0x0001` | `ntfs.FlagInUse` | The record is allocated to a file. |
| `0x0002` | `ntfs.FlagDirectory` | The record describes a directory. |

### Fixed system records

The first twelve `$mft` records are reserved. `ntfs.SystemName` maps an index to
its name; anything else is a user file.

| Index | Name | Index | Name |
| --- | --- | --- | --- |
| 0 | `$mft` | 6 | `$bitmap` |
| 1 | `$mftmirr` | 7 | `$boot` |
| 2 | `$logfile` | 8 | `$badclus` |
| 3 | `$volume` | 9 | `$secure` |
| 4 | `$attrdef` | 10 | `$upcase` |
| 5 | `.` (root) | 11 | `$extend` |

## Attribute header

Every attribute starts with a common header. Decoded by `decodeAttribute` through
`ntfs.Attributes` (an iterator) or `ntfs.FindAttribute`.

| Offset | Size | Field | Notes |
| --- | --- | --- | --- |
| `0x00` | 4 | Type | See [attribute types](#attribute-types). |
| `0x04` | 4 | Length | Total on-disk length, header included. |
| `0x08` | 1 | Non-resident flag | Non-zero means the value lives outside the record. |
| `0x09` | 1 | Name length | Zero for an unnamed attribute. |
| `0x0A` | 2 | Flags | Compressed, encrypted, sparse. |
| `0x0C` | 2 | Attribute ID | Identifies the attribute within its record. |

The list ends at a type of `0xFFFFFFFF`.

### Resident attributes

| Offset | Size | Field | Notes |
| --- | --- | --- | --- |
| `0x10` | 4 | Content size | Byte length of the value. |
| `0x14` | 2 | Value offset | Start of the value within the record. |

### Non-resident attributes

| Offset | Size | Field | Notes |
| --- | --- | --- | --- |
| `0x10` | 8 | Start VCN | First cluster covered. |
| `0x18` | 8 | Last VCN | Last cluster covered. |
| `0x20` | 2 | Runlist offset | Relative to the attribute header. |
| `0x22` | 2 | Compression unit size | Meaningful only for compressed values. |
| `0x28` | 8 | Allocated size | Bytes occupied on disk. |
| `0x38` | 8 | Content size | Logical length in bytes. |

The non-resident header is `0x40` bytes, so the runlist offset must be at least
`0x40` and less than the attribute length.

### Attribute types

| Value | Constant | Meaning |
| --- | --- | --- |
| `0x10` | `ntfs.TypeStandardInformation` | `$STANDARD_INFORMATION` |
| `0x30` | `ntfs.TypeFileName` | `$FILE_NAME` |
| `0x80` | `ntfs.TypeData` | `$DATA` — the unnamed payload of a file |

Only these three are recognised; `FindAttribute` returns
`ntfs.ErrAttributeNotFound` for anything else.

## Runlists

A runlist is a sequence of variable-width entries describing contiguous spans of
clusters. Decoded by `ntfs.ParseRunList`. Each entry:

1. **Header byte** — low nibble = width in bytes of the *length* field; high
   nibble = width in bytes of the *offset* field. A header of `0x00` ends the
   list. Either width being zero is an error.
2. **Length** — unsigned little-endian, the number of clusters in the run.
3. **Offset** — **signed** little-endian, the delta from the *previous run's*
   LCN. The running LCN is accumulated across entries.

The length field is unsigned and the offset field is signed; that is the whole
reason `signedField` exists. `ntfs.ClusterAt` maps a runlist-relative cluster
index back to a volume cluster.

### Worked examples

`$mft` record 0 on a fresh 64 MiB volume:

```
11 07 04
```

- `0x11` — one length byte, one offset byte
- `0x07` — length: 7 clusters
- `0x04` — delta: `+4` → run at cluster 4

Result: `[]Run{{FirstCluster: 4, Length: 7}}`.

`$mftmirr` record 1 on the same volume:

```
21 01 ff 1f
```

- `0x21` — one length byte, **two** offset bytes
- `0x01` — length: 1 cluster
- `0xFF 0x1F` — delta: `0x1FFF = +8191` → run at cluster 8191

A negative delta closes a gap left by a fragmented file:

```
11 01 fe
```

- length: 1 cluster, delta: `0xFE = -2` (the run sits two clusters *before* the
  previous one)

## Sentinel errors

Exported errors callers can match with `errors.Is`.

| Error | Package | Raised when |
| --- | --- | --- |
| `ErrNotNTFS` | `ntfs` | The boot sector signature is not `NTFS    `. |
| `ErrBadRecordMagic` | `ntfs` | An MFT record does not start with `FILE`. |
| `ErrAttributeNotFound` | `ntfs` | The requested attribute type is absent. |
| `ErrNoRuns` | `ntfs` | A runlist decoded to no runs. |
| `ErrRecordOutOfRange` | `volume` | A record index falls outside the `$mft` extent. |
| `ErrNoMFTExtent` | `volume` | Record 0 carries no non-resident `$DATA` attribute. |
| `ErrUnreadableStructure` | `check` | A structure cannot be judged. |
| `ErrNothingToRepair` | `repair` | The volume is consistent. |
| `ErrNotImplemented` | `repair` | A finding has no modelled fix. |
| `ErrReadOnly` | `device` | A write was attempted on a read-only handle. |

## Known limitations

- **Update sequence (fixup) arrays are not applied.** Records are compared raw,
  which is exactly what the `$mftmirr` check needs. A structure whose payload
  crosses a sector boundary and contains the update sequence words is decoded as
  stored, not as after fixup. Records 0–3 and their `$DATA` runlists are far from
  the boundary, so current checks are unaffected.
- **Only the unnamed `$DATA` stream is followed.** Named data streams are not
  decoded.
- **`RecordCount` reports the allocated extent**, derived from the runlist length,
  not the logical `$DATA` content size. The two can differ by the tail of the last
  cluster.
