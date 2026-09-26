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
	paths := strings.Fields(stdout.String())
	if len(paths) == 0 || len(paths)%2 != 0 {
		t.Fatalf("output paths are not image/text pairs: %v", paths)
	}
	for index := 0; index < len(paths); index += 2 {
		imagePath := paths[index]
		textPath := paths[index+1]
		if !strings.HasPrefix(filepath.Base(imagePath), "preview-page-") {
			t.Fatalf("unexpected image path: %s", imagePath)
		}
		if _, err := os.Stat(imagePath); err != nil {
			t.Fatalf("rendered image does not exist: %v", err)
		}
		if strings.TrimSuffix(imagePath, filepath.Ext(imagePath))+".txt" != textPath {
			t.Fatalf("text path %q does not match image path %q", textPath, imagePath)
		}
		content, err := os.ReadFile(textPath)
		if err != nil {
			t.Fatalf("extracted text does not exist: %v", err)
		}
		if len(content) == 0 {
			t.Fatalf("extracted text is empty: %s", textPath)
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

func TestRunRejectsNegativeMaxPages(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Run(context.Background(), []string{"--max-pages", "-1", "input.pdf", t.TempDir()}, &stdout, &stderr); exitCode != 1 {
		t.Fatalf("unexpected exit code: %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "max pages must not be negative") {
		t.Fatalf("unexpected error: %s", stderr.String())
	}
}

func TestRunRejectsNegativeMaxCharacters(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Run(context.Background(), []string{"--max-characters", "-1", "input.pdf", t.TempDir()}, &stdout, &stderr); exitCode != 1 {
		t.Fatalf("unexpected exit code: %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "max characters must not be negative") {
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
