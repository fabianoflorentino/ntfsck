package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabianoflorentino/ntfsck/internal/volumeimg"
)

func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("check exits 0 on a consistent volume", func(t *testing.T) {
		t.Parallel()

		stdout, stderr, code := run(t, "--check", writeImage(t, volumeimg.New()))

		if code != 0 {
			t.Fatalf("exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
		}

		if !strings.Contains(stdout, "Filesystem is consistent.") {
			t.Errorf("stdout missing the clean verdict:\n%s", stdout)
		}
	})

	t.Run("check exits 3 on an inconsistent volume", func(t *testing.T) {
		t.Parallel()

		stdout, stderr, code := run(t, "--check", writeImage(t, volumeimg.NewDiverged()))

		if code != 3 {
			t.Fatalf("exit code = %d, want 3\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
		}

		if !strings.Contains(stdout, "record 3 ($volume) differs from $mft") {
			t.Errorf("stdout missing the divergence:\n%s", stdout)
		}

		if stderr != "" {
			t.Errorf("stderr = %q, want empty on a successful check", stderr)
		}
	})

	t.Run("json output carries the divergence", func(t *testing.T) {
		t.Parallel()

		stdout, _, code := run(t, "--check", "--output", "json", writeImage(t, volumeimg.NewDiverged()))

		if code != 3 {
			t.Fatalf("exit code = %d, want 3", code)
		}

		var got struct {
			Consistent bool `json:"consistent"`
			Findings   []struct {
				Structure string `json:"structure"`
				Severity  string `json:"severity"`
			} `json:"findings"`
		}

		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("Unmarshal(%s) error = %v", stdout, err)
		}

		if got.Consistent {
			t.Error("consistent = true, want false")
		}

		if len(got.Findings) != 5 {
			t.Fatalf("len(findings) = %d, want 5: %+v", len(got.Findings), got.Findings)
		}
	})

	t.Run("check never writes to the volume", func(t *testing.T) {
		t.Parallel()

		path := writeImage(t, volumeimg.NewDiverged())
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}

		if _, _, code := run(t, "--check", path); code != 3 {
			t.Fatalf("exit code = %d, want 3", code)
		}

		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}

		if !bytes.Equal(before, after) {
			t.Error("the volume changed during --check, want read-only behaviour")
		}
	})

	t.Run("repair plans without writing", func(t *testing.T) {
		t.Parallel()

		path := writeImage(t, volumeimg.NewDiverged())
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}

		stdout, stderr, code := run(t, "--repair", "--dry-run", path)

		if code != 3 {
			t.Fatalf("exit code = %d, want 3\nstderr:\n%s", code, stderr)
		}

		if !strings.Contains(stdout, "cluster 64") {
			t.Errorf("stdout missing the planned action:\n%s", stdout)
		}

		if !strings.Contains(stdout, "writing is not enabled yet") {
			t.Errorf("stdout missing the safety notice:\n%s", stdout)
		}

		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}

		if !bytes.Equal(before, after) {
			t.Error("the volume changed during --repair --dry-run")
		}
	})

	t.Run("dump-mft prints a record header", func(t *testing.T) {
		t.Parallel()

		stdout, stderr, code := run(t, "--dump-mft", "--mft-record", "0", writeImage(t, volumeimg.New()))

		if code != 0 {
			t.Fatalf("exit code = %d, want 0\nstderr:\n%s", code, stderr)
		}

		for _, want := range []string{"record 0", "$mft", "in use:       true"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("stdout missing %q:\n%s", want, stdout)
			}
		}
	})

	t.Run("rejects a missing device argument", func(t *testing.T) {
		t.Parallel()

		stdout, stderr, code := run(t, "--check")

		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}

		if !strings.Contains(stderr, "device or image path is required") {
			t.Errorf("stderr = %q, want a usage message", stderr)
		}

		if stdout != "" {
			t.Errorf("stdout = %q, want empty", stdout)
		}
	})

	t.Run("rejects an unknown output format", func(t *testing.T) {
		t.Parallel()

		_, stderr, code := run(t, "--check", "--output", "yaml", writeImage(t, volumeimg.New()))

		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}

		if !strings.Contains(stderr, "unknown output format") {
			t.Errorf("stderr = %q, want a format error", stderr)
		}
	})

	t.Run("rejects mutually exclusive actions", func(t *testing.T) {
		t.Parallel()

		_, stderr, code := run(t, "--check", "--repair", writeImage(t, volumeimg.New()))

		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}

		if !strings.Contains(stderr, "none of the others can be") {
			t.Errorf("stderr = %q, want a mutual-exclusion error", stderr)
		}
	})

	t.Run("rejects a non-ntfs volume", func(t *testing.T) {
		t.Parallel()

		_, stderr, code := run(t, "--check", writeImage(t, volumeimg.NewEmpty()))

		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}

		if !strings.Contains(stderr, "does not carry an ntfs boot sector") {
			t.Errorf("stderr = %q, want a boot sector error", stderr)
		}
	})
}

func run(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()

	var out, errOut bytes.Buffer

	code = Run(args, &out, &errOut)

	return out.String(), errOut.String(), code
}

func writeImage(t *testing.T, img *volumeimg.Bytes) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "volume.img")
	if err := os.WriteFile(path, img.Bytes(), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}

	return path
}
