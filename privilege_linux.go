//go:build linux

package svcforge

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// IsAdministrator reports whether the current process has system-level installation privileges.
func IsAdministrator() bool {
	return os.Geteuid() == 0
}

// ReexecElevated starts the current executable with administrator privileges and waits for it.
// It is intended for lifecycle CLI operations only; application secrets should not be passed in argv.
func ReexecElevated(args []string) (int, error) {
	if IsAdministrator() {
		return 0, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return -1, fmt.Errorf("resolve current executable: %w", err)
	}
	var name string
	var commandArgs []string
	switch {
	case commandAvailable("sudo"):
		name = "sudo"
		commandArgs = append([]string{"--", exe}, args...)
	case commandAvailable("doas"):
		name = "doas"
		commandArgs = append([]string{exe}, args...)
	case commandAvailable("pkexec"):
		name = "pkexec"
		commandArgs = append([]string{exe}, args...)
	default:
		return -1, errors.New("administrator privileges are required and no supported elevation tool (sudo, doas, pkexec) is available")
	}
	cmd := exec.Command(name, commandArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return -1, fmt.Errorf("run elevation tool %s: %w", name, err)
	}
	return 0, nil
}

func commandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
