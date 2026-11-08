//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package renderer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

type libreOfficeExecutionResult struct {
	stdout string
	stderr string
	err    error
}

func TestExecuteLibreOfficeCancelsDescendants(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "descendant.pid")
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, _, err := executeLibreOffice(ctx, os.Args[0], libreOfficeHelperArguments("parent", pidPath))
		result <- err
	}()

	descendantPID := waitForHelperPID(t, pidPath)
	t.Cleanup(func() { _ = syscall.Kill(descendantPID, syscall.SIGKILL) })
	cancel()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected cancellation error")
		}
	case <-time.After(3 * time.Second):
		_ = syscall.Kill(descendantPID, syscall.SIGKILL)
		<-result
		t.Fatal("LibreOffice execution did not return after cancellation")
	}

	waitForProcessExit(t, descendantPID)
}

func TestExecuteLibreOfficeDeadlineStopsDescendants(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "descendant.pid")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	result := runLibreOfficeHelper(ctx, pidPath)

	descendantPID := waitForHelperPID(t, pidPath)
	t.Cleanup(func() { _ = syscall.Kill(descendantPID, syscall.SIGKILL) })
	waitForExecutionResult(t, result)
	waitForProcessExit(t, descendantPID)
}

func TestExecuteLibreOfficeAllowsSuccessfulCommand(t *testing.T) {
	stdout, stderr, err := executeLibreOffice(
		context.Background(),
		os.Args[0],
		libreOfficeHelperArguments("success", filepath.Join(t.TempDir(), "unused.pid")),
	)
	if err != nil {
		t.Fatalf("execute successful command: %v", err)
	}
	if !strings.Contains(stdout, "helper success\n") {
		t.Fatalf("stdout = %q, want success output", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty output", stderr)
	}
}

func TestExecuteLibreOfficeStopsDescendantHoldingOutputPipes(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "descendant.pid")
	result := make(chan libreOfficeExecutionResult, 1)
	go func() {
		stdout, stderr, err := executeLibreOffice(
			context.Background(),
			os.Args[0],
			libreOfficeHelperArguments("parent-exits", pidPath),
		)
		result <- libreOfficeExecutionResult{stdout: stdout, stderr: stderr, err: err}
	}()

	descendantPID := waitForHelperPID(t, pidPath)
	t.Cleanup(func() { _ = syscall.Kill(descendantPID, syscall.SIGKILL) })
	select {
	case execution := <-result:
		if !errors.Is(execution.err, exec.ErrWaitDelay) {
			t.Fatalf("error = %v, want exec.ErrWaitDelay", execution.err)
		}
		if !strings.Contains(execution.stdout, "helper child ready") {
			t.Fatalf("stdout = %q, want child diagnostics", execution.stdout)
		}
		if !strings.Contains(execution.stderr, "helper parent exiting") {
			t.Fatalf("stderr = %q, want parent diagnostics", execution.stderr)
		}
	case <-time.After(5 * time.Second):
		_ = syscall.Kill(descendantPID, syscall.SIGKILL)
		t.Fatal("LibreOffice execution did not return after WaitDelay")
	}

	waitForProcessExit(t, descendantPID)
}

func TestExecuteLibreOfficeCancellationDoesNotAffectConcurrentCommand(t *testing.T) {
	firstPIDPath := filepath.Join(t.TempDir(), "first.pid")
	secondPIDPath := filepath.Join(t.TempDir(), "second.pid")
	firstContext, cancelFirst := context.WithCancel(context.Background())
	secondContext, cancelSecond := context.WithCancel(context.Background())
	defer cancelFirst()
	defer cancelSecond()
	firstResult := runLibreOfficeHelper(firstContext, firstPIDPath)
	secondResult := runLibreOfficeHelper(secondContext, secondPIDPath)

	firstPID := waitForHelperPID(t, firstPIDPath)
	secondPID := waitForHelperPID(t, secondPIDPath)
	t.Cleanup(func() { _ = syscall.Kill(firstPID, syscall.SIGKILL) })
	t.Cleanup(func() { _ = syscall.Kill(secondPID, syscall.SIGKILL) })
	cancelFirst()
	waitForExecutionResult(t, firstResult)
	waitForProcessExit(t, firstPID)
	if !processExists(secondPID) {
		cancelSecond()
		waitForExecutionResult(t, secondResult)
		t.Fatal("canceling one command terminated a concurrent command")
	}

	cancelSecond()
	waitForExecutionResult(t, secondResult)
	waitForProcessExit(t, secondPID)
}

func TestLibreOfficeProcessHelper(t *testing.T) {
	separator := -1
	for index, argument := range os.Args {
		if argument == "--" {
			separator = index
			break
		}
	}
	if separator < 0 {
		return
	}
	if separator+2 >= len(os.Args) {
		t.Fatal("missing helper arguments")
	}

	mode := os.Args[separator+1]
	pidPath := os.Args[separator+2]
	if mode == "success" {
		fmt.Fprintln(os.Stdout, "helper success")
		return
	}
	if mode == "child" {
		if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintln(os.Stdout, "helper child ready")
		waitForHelperSignal()
		return
	}
	if mode != "parent" && mode != "parent-exits" {
		t.Fatalf("unknown helper mode %q", mode)
	}

	child := exec.Command(os.Args[0], libreOfficeHelperArguments("child", pidPath)...)
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	if mode == "parent-exits" {
		waitForHelperPID(t, pidPath)
		fmt.Fprintln(os.Stderr, "helper parent exiting")
		if err := child.Process.Release(); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := child.Wait(); err != nil {
		t.Fatal(err)
	}
}

func libreOfficeHelperArguments(mode, pidPath string) []string {
	return []string{"-test.run=^TestLibreOfficeProcessHelper$", "--", mode, pidPath}
}

func waitForHelperSignal() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
}

func runLibreOfficeHelper(ctx context.Context, pidPath string) <-chan error {
	result := make(chan error, 1)
	go func() {
		_, _, err := executeLibreOffice(ctx, os.Args[0], libreOfficeHelperArguments("parent", pidPath))
		result <- err
	}()
	return result
}

func waitForExecutionResult(t *testing.T, result <-chan error) {
	t.Helper()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected command error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("command did not return")
	}
}

func waitForHelperPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(string(data))
			if parseErr == nil {
				return pid
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("read helper PID: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("helper process did not start")
	return 0
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

func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || !errors.Is(err, syscall.ESRCH)
}
