//go:build !aix && !android && !darwin && !dragonfly && !freebsd && !illumos && !ios && !linux && !netbsd && !openbsd && !solaris

package renderer

import "os/exec"

func configureLibreOfficeProcessGroup(_ *exec.Cmd) {}

func terminateLibreOfficeProcessGroup(_ *exec.Cmd) error { return nil }
