package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fabianoflorentino/ntfsck/internal/report"
)

// version is injected at build time with -ldflags "-X main.version=...".
var version = "dev"

const longDescription = `ntfsck verifies and repairs NTFS volumes on Linux.

Analysis is read-only unless a repair is requested, and the current build plans
repairs without writing them.`

// NewRootCommand builds the ntfsck command tree.
func NewRootCommand() *cobra.Command {
	var (
		doCheck      bool
		doRepair     bool
		dryRun       bool
		dumpMFT      bool
		mftIndex     int64
		outputFormat string
	)

	cmd := &cobra.Command{
		Use:           "ntfsck [flags] <device>",
		Short:         "Verify and repair NTFS volumes",
		Long:          longDescription,
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
		RunE: func(cmd *cobra.Command, args []string) error {
			devicePath, err := deviceArg(args)
			if err != nil {
				return err
			}

			format, err := parseFormat(outputFormat)
			if err != nil {
				return err
			}

			ctx, stop := signalContext()
			defer stop()

			code := ExitOK

			switch {
			case doCheck:
				code, err = runCheck(ctx, cmd.OutOrStdout(), devicePath, string(format))
			case doRepair:
				code, err = runRepair(ctx, cmd.OutOrStdout(), devicePath, string(format), dryRun)
			case dumpMFT:
				code, err = runDumpMFT(ctx, cmd.OutOrStdout(), devicePath, mftIndex)
			}

			if err != nil {
				return err
			}

			if code != ExitOK {
				return &exitError{code: code}
			}

			return nil
		},
	}

	flags := cmd.Flags()
	flags.BoolVar(&doCheck, "check", false, "analyze the volume without writing anything")
	flags.BoolVar(&doRepair, "repair", false, "plan the repairs the volume needs")
	flags.BoolVar(&dryRun, "dry-run", false, "with --repair, plan only and never open the volume for writing")
	flags.BoolVar(&dumpMFT, "dump-mft", false, "print one MFT record header")
	flags.Int64Var(&mftIndex, "mft-record", 0, "MFT record index to print with --dump-mft")
	flags.StringVar(&outputFormat, "output", string(report.FormatText), outputFlagUsage())

	// The actions are mutually exclusive by nature: a scan that also writes is
	// exactly the ambiguity this tool exists to remove.
	cmd.MarkFlagsMutuallyExclusive("check", "repair", "dump-mft")
	cmd.MarkFlagsMutuallyExclusive("dry-run", "check", "dump-mft")

	return cmd
}

// exitError carries a non-zero exit code up to main without printing a message,
// because the report on stdout already explained the outcome.
type exitError struct {
	code int
}

func (e *exitError) Error() string {
	return fmt.Sprintf("ntfsck finished with exit code %d", e.code)
}

// ExitCode reports the process exit code the error asks for.
func (e *exitError) ExitCode() int { return e.code }

func deviceArg(args []string) (string, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return "", errUsage("a device or image path is required")
	}

	return args[0], nil
}

func parseFormat(value string) (report.Format, error) {
	format := report.Format(value)
	if !format.Valid() {
		return "", errUsage(fmt.Sprintf(
			"unknown output format %q, want one of %v", value, report.Formats(),
		))
	}

	return format, nil
}

func outputFlagUsage() string {
	names := make([]string, 0, len(report.Formats()))
	for _, f := range report.Formats() {
		names = append(names, string(f))
	}

	return "report format: " + strings.Join(names, "|")
}

type usageError struct {
	msg string
}

func (e *usageError) Error() string { return e.msg }

func errUsage(msg string) error { return &usageError{msg: msg} }
