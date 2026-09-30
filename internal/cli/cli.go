// Package cli is the primary adapter: it turns command-line arguments into calls
// on the check and repair use cases and renders their output.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/fabianoflorentino/ntfsck/internal/check"
	"github.com/fabianoflorentino/ntfsck/internal/device"
	"github.com/fabianoflorentino/ntfsck/internal/ntfs"
	"github.com/fabianoflorentino/ntfsck/internal/repair"
	"github.com/fabianoflorentino/ntfsck/internal/report"
	"github.com/fabianoflorentino/ntfsck/internal/volume"
)

// Exit codes follow the Unix convention: 0 for success, 2 for a usage mistake,
// and dedicated codes so a caller can tell "the volume is broken" from "ntfsck
// could not run".
const (
	// ExitOK means the volume is consistent or the request succeeded.
	ExitOK = 0
	// ExitFailure means ntfsck could not complete the request.
	ExitFailure = 1
	// ExitUsage means the command line was wrong.
	ExitUsage = 2
	// ExitInconsistent means the check completed and found problems.
	ExitInconsistent = 3
)

// signalContext returns a context cancelled on SIGINT or SIGTERM so a long scan
// stops at the next checkpoint instead of dying mid-write.
func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// openVolume is the single place where an adapter is attached to the port, which
// is what keeps the domain free of device handling.
func openVolume(path string, readOnly bool) (*volume.Volume, error) {
	src, err := device.Open(path, readOnly)
	if err != nil {
		return nil, err
	}

	vol, err := volume.Open(src)
	if err != nil {
		return nil, errors.Join(err, src.Close())
	}

	return vol, nil
}

// closeVolume releases a volume at the end of a command. A close failure is
// reported to stderr rather than returned, because these commands already have
// an exit code by the time cleanup runs.
func closeVolume(vol *volume.Volume) {
	if err := vol.Close(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "ntfsck:", err)
	}
}

// runCheck is the read-only use case: open, inspect, report.
func runCheck(ctx context.Context, out io.Writer, path, outputFormat string) (int, error) {
	vol, err := openVolume(path, true)
	if err != nil {
		return ExitFailure, err
	}
	defer closeVolume(vol)

	result, err := check.New(vol).Run(ctx)
	if err != nil {
		return ExitFailure, err
	}

	if err := report.Write(out, result, report.Format(outputFormat)); err != nil {
		return ExitFailure, err
	}

	if !result.Consistent() {
		return ExitInconsistent, nil
	}

	return ExitOK, nil
}

// runRepair derives a plan and prints it. Nothing is written: the repair engine
// only gains an apply step once every fix is modelled and tested.
func runRepair(ctx context.Context, out io.Writer, path, outputFormat string, dryRun bool) (int, error) {
	vol, err := openVolume(path, !dryRun)
	if err != nil {
		return ExitFailure, err
	}
	defer closeVolume(vol)

	result, err := check.New(vol).Run(ctx)
	if err != nil {
		return ExitFailure, err
	}

	plan, err := repair.New(vol).Plan(ctx, result)
	if err != nil {
		return ExitFailure, fmt.Errorf("planning repair: %w", err)
	}

	if plan.Empty() {
		if err := report.Write(out, result, report.Format(outputFormat)); err != nil {
			return ExitFailure, err
		}

		return ExitOK, nil
	}

	for _, action := range plan.Actions {
		if _, err := fmt.Fprintln(out, action); err != nil {
			return ExitFailure, err
		}
	}

	_, err = fmt.Fprintf(out, "\n%d action(s) planned; writing is not enabled yet.\n", len(plan.Actions))

	return ExitInconsistent, err
}

// runDumpMFT prints one MFT record header for manual inspection.
func runDumpMFT(ctx context.Context, out io.Writer, path string, index int64) (int, error) {
	vol, err := openVolume(path, true)
	if err != nil {
		return ExitFailure, err
	}
	defer closeVolume(vol)

	if err := ctx.Err(); err != nil {
		return ExitFailure, fmt.Errorf("dumping mft: %w", err)
	}

	header, err := vol.MFTRecord(index)
	if err != nil {
		return ExitFailure, err
	}

	name := ntfs.SystemName(uint64(index))
	if name == "" {
		name = "(user file)"
	}

	_, err = fmt.Fprintf(
		out,
		"record %d  %s\n  magic:        %s\n  sequence:     %d\n  in use:       %t\n  directory:    %t\n  hard links:   %d\n  attrs at:     %d\n  bytes in use: %d\n",
		index, name, header.Magic, header.SequenceNumber, header.IsInUse(),
		header.IsDirectory(), header.HardLinkCount, header.AttributesOffset, header.BytesInUse,
	)

	return ExitOK, err
}
