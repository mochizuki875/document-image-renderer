//go:build linux

package integration_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mochizuki875/document-image-renderer/pkg/renderer"
)

func TestLibreOfficeTimeoutTerminatesStoppedDescendant(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_INTEGRATION_TESTS=1 to run document conversions")
	}
	executable := libreOfficeExecutable(t)
	pidsPath := filepath.Join(t.TempDir(), "libreoffice.pids")
	wrapperPath := filepath.Join(t.TempDir(), "libreoffice-wrapper")
	wrapper := `#!/bin/sh
"$REAL_LIBREOFFICE" "$@" &
child_pid=$!
printf '%s %s\n' "$$" "$child_pid" > "$LIBREOFFICE_PIDS"
kill -STOP "$child_pid"
wait "$child_pid"
`
	if err := os.WriteFile(wrapperPath, []byte(wrapper), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REAL_LIBREOFFICE", executable)
	t.Setenv("LIBREOFFICE_PIDS", pidsPath)
	t.Cleanup(func() {
		data, readErr := os.ReadFile(pidsPath)
		if readErr != nil {
			return
		}
		fields := strings.Fields(string(data))
		if len(fields) == 0 {
			return
		}
		groupID, parseErr := strconv.Atoi(fields[0])
		if parseErr == nil {
			_ = syscall.Kill(-groupID, syscall.SIGKILL)
		}
	})

	options := renderer.DefaultRenderOptions()
	options.LibreOfficeExecutable = wrapperPath
	options.LibreOfficeTimeout = 500 * time.Millisecond
	started := time.Now()
	_, err := renderer.RenderDocument(
		context.Background(),
		filepath.Join("..", "documents", "samplefile.xlsm"),
		t.TempDir(),
		&options,
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("conversion returned after %s, want at most 5s", elapsed)
	}

	pids := readProcessIDs(t, pidsPath)
	for _, pid := range pids {
		waitForProcessExit(t, pid)
	}
}

func libreOfficeExecutable(t *testing.T) string {
	t.Helper()
	for _, candidate := range []string{"libreoffice", "soffice"} {
		executable, err := exec.LookPath(candidate)
		if err == nil {
			return executable
		}
	}
	t.Skip("LibreOffice is not installed")
	return ""
}

func readProcessIDs(t *testing.T, path string) []int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read process IDs: %v", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) != 2 {
		t.Fatalf("process IDs = %q, want parent and child", data)
	}
	pids := make([]int, 0, len(fields))
	for _, field := range fields {
		pid, err := strconv.Atoi(field)
		if err != nil {
			t.Fatalf("parse process ID %q: %v", field, err)
		}
		pids = append(pids, pid)
	}
	return pids
}

func waitForProcessExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %d is still running", pid)
}
