// Command ntfsck verifies and repairs NTFS volumes on Linux.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/fabianoflorentino/ntfsck/internal/cli"
)

// exitCoder lets a use case choose the process exit code without printing a
// second message over the report already on stdout.
type exitCoder interface {
	ExitCode() int
}

func main() {
	os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr))
}

// Run executes one ntfsck invocation and returns the process exit code. Keeping
// it separate from main is what makes the whole command testable in-process.
func Run(args []string, stdout, stderr io.Writer) int {
	cmd := cli.NewRootCommand()
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err := cmd.Execute()
	if err == nil {
		return cli.ExitOK
	}

	var coder exitCoder
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}

	// The failure is reported on stderr; if even that write fails there is
	// nothing left to report it to.
	_, _ = fmt.Fprintln(stderr, "ntfsck:", err)

	return cli.ExitFailure
}
