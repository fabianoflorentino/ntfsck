package check_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fabianoflorentino/ntfsck/internal/check"
	"github.com/fabianoflorentino/ntfsck/internal/ntfs"
	"github.com/fabianoflorentino/ntfsck/internal/volume"
	"github.com/fabianoflorentino/ntfsck/internal/volumeimg"
)

func TestCheckerRun(t *testing.T) {
	t.Parallel()

	t.Run("reports every mirrored record as matching", func(t *testing.T) {
		t.Parallel()

		report := runCheck(t, volumeimg.New())

		if report.RecordCount != volumeimg.RecordCount {
			t.Errorf("RecordCount = %d, want %d", report.RecordCount, volumeimg.RecordCount)
		}

		if report.ClusterSize != volumeimg.ClusterSize {
			t.Errorf("ClusterSize = %d, want %d", report.ClusterSize, volumeimg.ClusterSize)
		}

		if !report.Consistent() {
			t.Errorf("Consistent() = false, want true; findings = %+v", report.Findings)
		}
	})

	t.Run("detects a single diverged mirrored record", func(t *testing.T) {
		t.Parallel()

		report := runCheck(t, volumeimg.NewDiverged())

		if report.Consistent() {
			t.Fatal("Consistent() = true, want false")
		}

		want := []check.Finding{
			{
				Structure: "$mftmirr",
				Severity:  check.SeverityOK,
				Detail:    "record 0 matches $mft",
			},
			{
				Structure: "$mftmirr",
				Severity:  check.SeverityOK,
				Detail:    "record 1 matches $mft",
			},
			{
				Structure: "$mftmirr",
				Severity:  check.SeverityOK,
				Detail:    "record 2 matches $mft",
			},
			{
				Structure: "$mftmirr",
				Severity:  check.SeverityWarn,
				Detail:    "record 3 ($volume) differs from $mft",
			},
			{
				Structure: "$mftmirr",
				Severity:  check.SeverityWarn,
				Detail:    "1 of 4 mirrored records diverge from $mft",
			},
		}

		got := mirrorFindings(report)

		if len(got) != len(want) {
			t.Fatalf("len($mftmirr findings) = %d, want %d: %+v", len(got), len(want), got)
		}

		for i, expected := range want {
			if got[i] != expected {
				t.Errorf("$mftmirr findings[%d] = %+v, want %+v", i, got[i], expected)
			}
		}
	})

	t.Run("refuses to certify a volume it cannot read", func(t *testing.T) {
		t.Parallel()

		if _, err := volume.Open(volumeimg.NewEmpty().Section()); !errors.Is(err, ntfs.ErrNotNTFS) {
			t.Errorf("volume.Open() error = %v, want ntfs.ErrNotNTFS", err)
		}
	})
}

func TestCheckerRunHonoursCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	vol := openVolume(t, volumeimg.New())

	if _, err := check.New(vol).Run(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}

func TestReportConsistent(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		findings []check.Finding
		want     bool
	}{
		{"no findings", nil, true},
		{
			name:     "only ok findings",
			findings: []check.Finding{{Severity: check.SeverityOK}},
			want:     true,
		},
		{
			name:     "a warning is enough",
			findings: []check.Finding{{Severity: check.SeverityWarn}},
			want:     false,
		},
		{
			name:     "an error is enough",
			findings: []check.Finding{{Severity: check.SeverityError}},
			want:     false,
		},
		{
			name:     "an unevaluated severity is not a pass",
			findings: []check.Finding{{Severity: check.SeverityUnknown}},
			want:     true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			report := check.Report{Findings: tt.findings}
			if got := report.Consistent(); got != tt.want {
				t.Errorf("Consistent() = %t, want %t", got, tt.want)
			}
		})
	}
}

func mirrorFindings(report check.Report) []check.Finding {
	var out []check.Finding
	for _, f := range report.Findings {
		if f.Structure == "$mftmirr" {
			out = append(out, f)
		}
	}

	return out
}

func runCheck(t *testing.T, img *volumeimg.Bytes) check.Report {
	t.Helper()

	report, err := check.New(openVolume(t, img)).Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	return report
}

func openVolume(t *testing.T, img *volumeimg.Bytes) *volume.Volume {
	t.Helper()

	vol, err := volume.Open(img.Section())
	if err != nil {
		t.Fatalf("volume.Open() error = %v", err)
	}

	t.Cleanup(func() {
		if err := vol.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	return vol
}
