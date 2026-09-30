package report_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/fabianoflorentino/ntfsck/internal/check"
	"github.com/fabianoflorentino/ntfsck/internal/report"
)

func TestFormatValid(t *testing.T) {
	t.Parallel()

	if !report.FormatText.Valid() || !report.FormatJSON.Valid() {
		t.Error("the built-in formats must be valid")
	}

	if report.Format("yaml").Valid() {
		t.Error("Format(\"yaml\").Valid() = true, want false")
	}
}

func TestWrite(t *testing.T) {
	t.Parallel()

	consistent := check.Report{
		Volume:      "/dev/sda1",
		ClusterSize: 4096,
		RecordCount: 1834921,
		Findings: []check.Finding{
			{Structure: "$mftmirr", Severity: check.SeverityOK, Detail: "record 0 matches $mft"},
		},
	}

	diverged := check.Report{
		Volume:      "/dev/sda1",
		ClusterSize: 4096,
		RecordCount: 1834921,
		Findings: []check.Finding{
			{Structure: "$mftmirr", Severity: check.SeverityWarn, Detail: "record 3 differs from $mft"},
		},
		NotCovered: []string{"$bitmap"},
	}

	t.Run("text reports a clean volume", func(t *testing.T) {
		t.Parallel()

		out := writeText(t, consistent)

		for _, want := range []string{
			"NTFS volume:  /dev/sda1",
			"Cluster size: 4096 bytes",
			"MFT records: 1834921",
			"[OK]  $mftmirr",
			"Filesystem is consistent.",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("text report missing %q:\n%s", want, out)
			}
		}
	})

	t.Run("text reports an inconsistent volume", func(t *testing.T) {
		t.Parallel()

		out := writeText(t, diverged)

		for _, want := range []string{
			"[WARN]  $mftmirr",
			"Filesystem is inconsistent. Repair recommended.",
			"Not checked yet: $bitmap",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("text report missing %q:\n%s", want, out)
			}
		}
	})

	t.Run("json carries every field", func(t *testing.T) {
		t.Parallel()

		out := writeJSON(t, diverged)

		var got struct {
			Volume      string `json:"volume"`
			ClusterSize int64  `json:"cluster_size"`
			RecordCount int64  `json:"record_count"`
			Consistent  bool   `json:"consistent"`
			Findings    []struct {
				Structure string `json:"structure"`
				Severity  string `json:"severity"`
				Detail    string `json:"detail"`
			} `json:"findings"`
			NotCovered []string `json:"not_covered"`
		}

		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("Unmarshal(%s) error = %v", out, err)
		}

		if got.Volume != "/dev/sda1" {
			t.Errorf("volume = %q, want /dev/sda1", got.Volume)
		}

		if got.Consistent {
			t.Error("consistent = true, want false")
		}

		if len(got.Findings) != 1 || got.Findings[0].Severity != "WARN" {
			t.Errorf("findings = %+v, want one WARN finding", got.Findings)
		}

		if len(got.NotCovered) != 1 || got.NotCovered[0] != "$bitmap" {
			t.Errorf("not_covered = %+v, want [$bitmap]", got.NotCovered)
		}
	})

	t.Run("json reports a clean volume as consistent", func(t *testing.T) {
		t.Parallel()

		out := writeJSON(t, consistent)

		var got struct {
			Consistent bool `json:"consistent"`
		}

		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("Unmarshal(%s) error = %v", out, err)
		}

		if !got.Consistent {
			t.Error("consistent = false, want true")
		}
	})
}

func writeText(t *testing.T, r check.Report) string {
	t.Helper()

	var b strings.Builder
	if err := report.Write(&b, r, report.FormatText); err != nil {
		t.Fatalf("Write(text) error = %v", err)
	}

	return b.String()
}

func writeJSON(t *testing.T, r check.Report) string {
	t.Helper()

	var b strings.Builder
	if err := report.Write(&b, r, report.FormatJSON); err != nil {
		t.Fatalf("Write(json) error = %v", err)
	}

	return b.String()
}
