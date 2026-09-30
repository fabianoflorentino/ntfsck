// Package repair is the write side of the hexagonal core. It turns read-only
// findings into an explicit plan first and only touches the medium once the
// operator has seen that plan.
package repair

import (
	"context"
	"errors"
	"fmt"

	"github.com/fabianoflorentino/ntfsck/internal/check"
	"github.com/fabianoflorentino/ntfsck/internal/ntfs"
)

// ErrNothingToRepair reports a volume the checker found consistent.
var ErrNothingToRepair = errors.New("repair: volume is consistent, nothing to repair")

// ErrNotImplemented reports a finding no repair action covers yet. ntfsck
// refuses to write rather than half-fix a structure it does not fully model.
var ErrNotImplemented = errors.New("repair: no action available yet")

// Volume is the read port the repair engine needs to locate what it must
// rewrite. Write access is a separate, narrower port declared by the code that
// applies a plan.
type Volume interface {
	MFTRecordBytes(index int64) ([]byte, error)
}

// Action is one metadata write the repair engine intends to perform.
type Action struct {
	// Structure is the NTFS structure the action targets.
	Structure string
	// Record is the MFT record index the action targets, or -1 when the
	// action covers a whole structure.
	Record int64
	// Detail describes the write in operator terms.
	Detail string
}

// String renders the action as a single plan line.
func (a Action) String() string {
	if a.Record < 0 {
		return fmt.Sprintf("%s: %s", a.Structure, a.Detail)
	}

	return fmt.Sprintf("%s record %d: %s", a.Structure, a.Record, a.Detail)
}

// Plan is the ordered set of actions a repair would apply.
type Plan struct {
	// Volume is the path or label the plan targets.
	Volume string
	// Actions are applied in order.
	Actions []Action
}

// Empty reports whether the plan would change nothing.
func (p Plan) Empty() bool { return len(p.Actions) == 0 }

// Repairer derives repair plans from findings. It never writes on its own; the
// caller applies the plan through a writable source.
type Repairer struct {
	volume Volume
}

// New returns a Repairer over vol.
func New(vol Volume) *Repairer {
	return &Repairer{volume: vol}
}

// Plan turns a check report into the ordered writes needed to make it
// consistent. Structures without a modelled fix return ErrNotImplemented so the
// caller cannot mistake a partial plan for a complete one.
func (r *Repairer) Plan(ctx context.Context, report check.Report) (Plan, error) {
	if err := ctx.Err(); err != nil {
		return Plan{}, fmt.Errorf("repair: %w", err)
	}

	if report.Consistent() {
		return Plan{Volume: report.Volume}, nil
	}

	plan := Plan{Volume: report.Volume}

	// One action per structure: the checker emits a finding per record, and a
	// repair is a single write covering the whole extent.
	planned := make(map[string]bool, len(report.Findings))

	for _, finding := range report.Findings {
		switch finding.Structure {
		case "$mftmirr":
			if planned[finding.Structure] {
				continue
			}

			action, err := r.planMirror(ctx, finding)
			if err != nil {
				return Plan{}, err
			}

			// A clean finding yields no action; keep looking for a divergent one.
			if action == nil {
				continue
			}

			planned[finding.Structure] = true
			plan.Actions = append(plan.Actions, *action)
		default:
			if planned[finding.Structure] {
				continue
			}

			return Plan{}, fmt.Errorf(
				"%w: %s (%s)", ErrNotImplemented, finding.Structure, finding.Detail,
			)
		}
	}

	return plan, nil
}

// planMirror turns a $mftmirr divergence into the single write that resolves it:
// refresh the mirror from the authoritative $mft.
func (r *Repairer) planMirror(ctx context.Context, finding check.Finding) (*Action, error) {
	if finding.Severity == check.SeverityOK {
		return nil, nil
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("repair: %w", err)
	}

	raw, err := r.volume.MFTRecordBytes(ntfs.MFTMirror)
	if err != nil {
		return nil, fmt.Errorf("repair: reading $mftmirr record: %w", err)
	}

	runs, _, err := ntfs.DataRuns(raw)
	if err != nil {
		return nil, fmt.Errorf("repair: locating $mftmirr payload: %w", err)
	}

	if len(runs) != 1 {
		return nil, fmt.Errorf(
			"%w: $mftmirr spans %d runs, refresh only supports a contiguous copy",
			ErrNotImplemented, len(runs),
		)
	}

	return &Action{
		Structure: "$mftmirr",
		Record:    -1,
		Detail: fmt.Sprintf(
			"rewrite the mirrored copy at cluster %d from $mft",
			runs[0].FirstCluster,
		),
	}, nil
}
