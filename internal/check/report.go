// Package check is the read-only side of the hexagonal core. It inspects a
// volume through a narrow port and reports findings; it never writes.
package check

import "strings"

// Severity ranks how much a finding should worry the operator.
type Severity int

const (
	// SeverityUnknown is the zero value so an unset Severity reads as
	// "not evaluated" rather than silently as "fine".
	SeverityUnknown Severity = iota
	// SeverityOK marks a structure that verified clean.
	SeverityOK
	// SeverityWarn marks an inconsistency that does not block mounting.
	SeverityWarn
	// SeverityError marks an inconsistency that makes the volume unusable.
	SeverityError
)

// String renders the severity as the short tag used in reports.
func (s Severity) String() string {
	switch s {
	case SeverityOK:
		return "OK"
	case SeverityWarn:
		return "WARN"
	case SeverityError:
		return "ERROR"
	case SeverityUnknown:
		return "UNKNOWN"
	default:
		return "UNKNOWN"
	}
}

// Finding is one verdict about one NTFS structure.
type Finding struct {
	// Structure is the NTFS structure examined, such as "$mftmirr".
	Structure string
	// Severity is the verdict.
	Severity Severity
	// Detail explains the verdict in one sentence.
	Detail string
}

// String renders the finding as a single report line.
func (f Finding) String() string {
	var b strings.Builder

	b.WriteString("[" + strings.ToUpper(f.Severity.String()) + "] ")
	b.WriteString(f.Structure)

	if f.Detail != "" {
		b.WriteString(": " + f.Detail)
	}

	return b.String()
}

// Report is the outcome of one read-only pass over a volume.
type Report struct {
	// Volume is the path or label that was inspected.
	Volume string
	// ClusterSize is the cluster size in bytes.
	ClusterSize int64
	// RecordCount is how many MFT records the volume holds.
	RecordCount int64
	// Findings holds one entry per structure examined, in report order.
	Findings []Finding
	// NotCovered names the structures this build does not examine yet. They are
	// reported separately from Findings so an unimplemented check can never be
	// mistaken for either a pass or a defect.
	NotCovered []string
}

// Consistent reports whether no finding reached SeverityWarn or above.
func (r Report) Consistent() bool {
	for _, f := range r.Findings {
		if f.Severity >= SeverityWarn {
			return false
		}
	}

	return true
}
