//go:build aix || android || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris

package renderer

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureLibreOfficeProcessGroup(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		err := killLibreOfficeProcessGroup(command)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}

func terminateLibreOfficeProcessGroup(command *exec.Cmd) error {
	err := killLibreOfficeProcessGroup(command)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func killLibreOfficeProcessGroup(command *exec.Cmd) error {
	if command.Process == nil {
		return os.ErrProcessDone
	}
	return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
}
