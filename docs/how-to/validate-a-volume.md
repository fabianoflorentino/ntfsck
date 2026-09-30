# How to validate ntfsck against a volume

**Goal:** run `ntfsck` against a real NTFS image, see a clean verdict, then
introduce a known fault and confirm `ntfsck` catches it.

**Done when:** `--check` exits `0` on the clean image and `3` on the corrupted
one, naming the diverging record.

This uses an image file, so it needs no root and cannot damage a real disk. It
requires `mkntfs` from `ntfs-3g`.

## Steps

### 1. Build the binary

```bash
make build          # writes bin/ntfsck
```

### 2. Create a throwaway NTFS image

```bash
mkdir -p /tmp/opencode
mkntfs -F -f -L testvol /tmp/opencode/real.img
```

`-F` forces a regular file, `-f` skips confirmation, `-L` sets the label.

### 3. Check the clean image

```bash
bin/ntfsck --check /tmp/opencode/real.img
echo "exit=$?"
```

Expect all four `$mftmirr` records to match and `exit=0`.

### 4. Inspect the structures by hand (optional)

Confirm the geometry and record 0 match the [on-disk reference](../reference/on-disk-formats.md):

```bash
bin/ntfsck --dump-mft --mft-record 0 /tmp/opencode/real.img
```

### 5. Corrupt the mirrored copy of record 3

`$mftmirr` holds its payload at the cluster named in **record 1**'s `$DATA`
runlist. On a fresh 64 MiB image that is cluster 8191 and the runlist is
`21 01 ff 1f`. Flip the mirrored record 3:

```bash
cp /tmp/opencode/real.img /tmp/opencode/corrupt.img
python3 - <<'PY'
off = 8191 * 4096 + 3 * 1024          # mirror payload + record 3
with open('/tmp/opencode/corrupt.img', 'r+b') as f:
    f.seek(off)
    f.write(b'XXXX')
PY
```

### 6. Check the corrupted image

```bash
bin/ntfsck --check /tmp/opencode/corrupt.img
echo "exit=$?"
```

Expect a `WARN` line for record 3 and `exit=3`.

### 7. Ask for the repair plan

```bash
bin/ntfsck --repair --dry-run /tmp/opencode/corrupt.img
```

Expect a single action naming the mirror cluster, and the reminder that writing
is not enabled.

## Validating against a real device

Only do this on a device you are prepared to lose. `--check` and
`--repair --dry-run` open read-only, but a bug must never be the thing that
teaches you the difference.

```bash
sudo bin/ntfsck --check /dev/sdXN
```

If you need root to read the device, prefer `sudo` on the command rather than
loosening device permissions.

## Automation

The same flow is viable in CI using an image file. A clean image must exit `0`:

```bash
mkntfs -F -f -L testvol "$img"
bin/ntfsck --check "$img"          # exit 0 on a clean volume
```

See [Add a check](add-a-check.md) if you want to validate a new check the same
way.
