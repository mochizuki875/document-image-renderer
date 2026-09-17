package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRendersPDF(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	outputDirectory := t.TempDir()
	source := filepath.Join("..", "..", "test", "documents", "samplefile.pdf")

	exitCode := Run(
		context.Background(),
		[]string{"--dpi", "72", "--prefix", "preview", source, outputDirectory},
		&stdout,
		&stderr,
	)
	if exitCode != 0 {
		t.Fatalf("unexpected exit code %d: %s", exitCode, stderr.String())
	}
	for _, path := range strings.Fields(stdout.String()) {
		if filepath.Base(path)[:7] != "preview" {
			t.Fatalf("unexpected output path: %s", path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("rendered image does not exist: %v", err)
		}
	}
}

func TestRunRejectsInvalidArguments(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Run(context.Background(), []string{"--dpi", "0", "input.pdf", t.TempDir()}, &stdout, &stderr); exitCode != 1 {
		t.Fatalf("unexpected exit code: %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "dpi must be between 1 and 1200") {
		t.Fatalf("unexpected error: %s", stderr.String())
	}
}

func TestRunRequiresSourceAndOutput(t *testing.T) {
	var output bytes.Buffer
	if exitCode := Run(context.Background(), nil, &output, &output); exitCode != 2 {
		t.Fatalf("unexpected exit code: %d", exitCode)
	}
	if !strings.Contains(output.String(), "Usage:") {
		t.Fatalf("usage was not printed: %s", output.String())
	}
}

func TestRunHelpSucceeds(t *testing.T) {
	var output bytes.Buffer
	if exitCode := Run(context.Background(), []string{"--help"}, &output, &output); exitCode != 0 {
		t.Fatalf("unexpected exit code: %d", exitCode)
	}
	if !strings.Contains(output.String(), "Usage:") {
		t.Fatalf("usage was not printed: %s", output.String())
	}
}
