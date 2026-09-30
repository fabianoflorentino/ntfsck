// Package report renders a check report for humans and for machines.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/fabianoflorentino/ntfsck/internal/check"
)

// Format selects the rendering of a report.
type Format string

const (
	// FormatText renders an operator-facing summary.
	FormatText Format = "text"
	// FormatJSON renders the report for machine consumption.
	FormatJSON Format = "json"
)

// Formats lists every supported rendering, for flag validation and help text.
func Formats() []Format { return []Format{FormatText, FormatJSON} }

// Valid reports whether f is a supported rendering.
func (f Format) Valid() bool {
	for _, candidate := range Formats() {
		if f == candidate {
			return true
		}
	}

	return false
}

// jsonFinding is the wire shape of a finding. Every field carries a tag so a
// rename cannot silently change the format.
type jsonFinding struct {
	Structure string `json:"structure"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

type jsonReport struct {
	Volume      string        `json:"volume"`
	ClusterSize int64         `json:"cluster_size"`
	RecordCount int64         `json:"record_count"`
	Consistent  bool          `json:"consistent"`
	Findings    []jsonFinding `json:"findings"`
	NotCovered  []string      `json:"not_covered"`
}

// Write renders r in the requested format.
func Write(w io.Writer, r check.Report, format Format) error {
	if format == FormatJSON {
		return writeJSON(w, r)
	}

	return writeText(w, r)
}

func writeJSON(w io.Writer, r check.Report) error {
	out := jsonReport{
		Volume:      r.Volume,
		ClusterSize: r.ClusterSize,
		RecordCount: r.RecordCount,
		Consistent:  r.Consistent(),
		Findings:    make([]jsonFinding, 0, len(r.Findings)),
		NotCovered:  r.NotCovered,
	}

	for _, f := range r.Findings {
		out.Findings = append(out.Findings, jsonFinding{
			Structure: f.Structure,
			Severity:  f.Severity.String(),
			Detail:    f.Detail,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("report: encoding json: %w", err)
	}

	return nil
}

func writeText(w io.Writer, r check.Report) error {
	if _, err := fmt.Fprintf(w, "NTFS volume:  %s\n", r.Volume); err != nil {
		return fmt.Errorf("report: writing volume name: %w", err)
	}

	if _, err := fmt.Fprintf(w, "Cluster size: %d bytes\nMFT records: %d\n\n", r.ClusterSize, r.RecordCount); err != nil {
		return fmt.Errorf("report: writing header: %w", err)
	}

	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, f := range r.Findings {
		if _, err := fmt.Fprintf(table, "[%s]\t%s\t%s\n", f.Severity, f.Structure, f.Detail); err != nil {
			return fmt.Errorf("report: writing findings: %w", err)
		}
	}
	if err := table.Flush(); err != nil {
		return fmt.Errorf("report: flushing findings: %w", err)
	}

	if len(r.NotCovered) > 0 {
		if _, err := fmt.Fprintf(w, "\nNot checked yet: %s\n", strings.Join(r.NotCovered, ", ")); err != nil {
			return fmt.Errorf("report: writing coverage note: %w", err)
		}
	}

	if r.Consistent() {
		if _, err := fmt.Fprintln(w, "\nFilesystem is consistent."); err != nil {
			return fmt.Errorf("report: writing verdict: %w", err)
		}

		return nil
	}

	if _, err := fmt.Fprintln(w, "\nFilesystem is inconsistent. Repair recommended."); err != nil {
		return fmt.Errorf("report: writing verdict: %w", err)
	}

	return nil
}
