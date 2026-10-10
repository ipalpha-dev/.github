package cli

import (
	"errors"
	"os"
	"os/exec"
)

// reexec runs another copy of the tool attached to this terminal and returns its exit code.
func reexec(bin string, args []string) error {
	c := exec.Command(bin, args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := c.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return exitCode(ee.ExitCode())
	}
	return err
}
