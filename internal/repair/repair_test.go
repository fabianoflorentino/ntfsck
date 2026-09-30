package repair_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fabianoflorentino/ntfsck/internal/check"
	"github.com/fabianoflorentino/ntfsck/internal/repair"
	"github.com/fabianoflorentino/ntfsck/internal/volume"
	"github.com/fabianoflorentino/ntfsck/internal/volumeimg"
)

func TestPlan(t *testing.T) {
	t.Parallel()

	t.Run("plans a mirror refresh for a diverged volume", func(t *testing.T) {
		t.Parallel()

		plan := planFor(t, volumeimg.NewDiverged())

		if plan.Empty() {
			t.Fatal("Empty() = true, want a plan")
		}

		if len(plan.Actions) != 1 {
			t.Fatalf("len(Actions) = %d, want 1: %+v", len(plan.Actions), plan.Actions)
		}

		action := plan.Actions[0]
		if action.Structure != "$mftmirr" {
			t.Errorf("Structure = %q, want $mftmirr", action.Structure)
		}

		if action.Record != -1 {
			t.Errorf("Record = %d, want -1 for a whole-structure action", action.Record)
		}

		if !strings.Contains(action.Detail, "cluster 64") {
			t.Errorf("Detail = %q, want it to name cluster 64", action.Detail)
		}
	})

	t.Run("plans nothing for a consistent volume", func(t *testing.T) {
		t.Parallel()

		plan := planFor(t, volumeimg.New())

		if !plan.Empty() {
			t.Errorf("Empty() = false, want an empty plan: %+v", plan.Actions)
		}
	})

	t.Run("refuses a structure it cannot model", func(t *testing.T) {
		t.Parallel()

		vol := open(t, volumeimg.NewDiverged())

		report := check.Report{
			Volume: "memory.img",
			Findings: []check.Finding{
				{Structure: "$bitmap", Severity: check.SeverityWarn, Detail: "clusters marked free are in use"},
			},
		}

		_, err := repair.New(vol).Plan(context.Background(), report)
		if !errors.Is(err, repair.ErrNotImplemented) {
			t.Errorf("Plan() error = %v, want ErrNotImplemented", err)
		}
	})

	t.Run("honours cancellation", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		vol := open(t, volumeimg.NewDiverged())

		_, err := repair.New(vol).Plan(ctx, check.Report{Volume: "memory.img"})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Plan() error = %v, want context.Canceled", err)
		}
	})
}

func TestActionString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		action repair.Action
		want   string
	}{
		{
			name:   "record scoped",
			action: repair.Action{Structure: "$mft", Record: 3, Detail: "reset header"},
			want:   "$mft record 3: reset header",
		},
		{
			name:   "structure scoped",
			action: repair.Action{Structure: "$mftmirr", Record: -1, Detail: "rewrite payload"},
			want:   "$mftmirr: rewrite payload",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.action.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func planFor(t *testing.T, img *volumeimg.Bytes) repair.Plan {
	t.Helper()

	vol := open(t, img)

	result, err := check.New(vol).Run(context.Background())
	if err != nil {
		t.Fatalf("check.Run() error = %v", err)
	}

	plan, err := repair.New(vol).Plan(context.Background(), result)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	return plan
}

func open(t *testing.T, img *volumeimg.Bytes) *volume.Volume {
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
