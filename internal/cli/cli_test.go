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

func TestRunRejectsNegativeRenderTimeout(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Run(context.Background(), []string{"--render-timeout", "-1", "input.pdf", t.TempDir()}, &stdout, &stderr); exitCode != 1 {
		t.Fatalf("unexpected exit code: %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "render timeout must not be negative") {
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

func TestRunRejectsUnknownFlag(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Run(context.Background(), []string{"--unknown", "input.pdf", t.TempDir()}, &stdout, &stderr); exitCode != 2 {
		t.Fatalf("unexpected exit code: %d", exitCode)
	}
}

func TestRunRejectsInvalidFormat(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Run(context.Background(), []string{"--format", "gif", "input.pdf", t.TempDir()}, &stdout, &stderr); exitCode != 1 {
		t.Fatalf("unexpected exit code: %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "image format") {
		t.Fatalf("unexpected error: %s", stderr.String())
	}
}

func TestRunRejectsInvalidJPEGQuality(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Run(context.Background(), []string{"--jpeg-quality", "101", "input.pdf", t.TempDir()}, &stdout, &stderr); exitCode != 1 {
		t.Fatalf("unexpected exit code: %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "jpeg quality") {
		t.Fatalf("unexpected error: %s", stderr.String())
	}
}

func TestRunRejectsTransparentJPEG(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Run(context.Background(), []string{"--format", "jpeg", "--transparent", "input.pdf", t.TempDir()}, &stdout, &stderr); exitCode != 1 {
		t.Fatalf("unexpected exit code: %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "transparent") {
		t.Fatalf("unexpected error: %s", stderr.String())
	}
}

func TestRunRejectsTraversalPrefix(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Run(context.Background(), []string{"--prefix", "../outside", "input.pdf", t.TempDir()}, &stdout, &stderr); exitCode != 1 {
		t.Fatalf("unexpected exit code: %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "filename prefix") {
		t.Fatalf("unexpected error: %s", stderr.String())
	}
}

func TestRunRejectsNegativeTimeout(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Run(context.Background(), []string{"--timeout", "-1", "input.pdf", t.TempDir()}, &stdout, &stderr); exitCode != 1 {
		t.Fatalf("unexpected exit code: %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "libreoffice timeout") {
		t.Fatalf("unexpected error: %s", stderr.String())
	}
}

func TestRunRejectsMissingSourceFile(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Run(context.Background(), []string{filepath.Join(t.TempDir(), "missing.pdf"), t.TempDir()}, &stdout, &stderr); exitCode != 1 {
		t.Fatalf("unexpected exit code: %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "does not exist") {
		t.Fatalf("unexpected error: %s", stderr.String())
	}
}

func TestRunRejectsUnsupportedExtension(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	source := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(source, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if exitCode := Run(context.Background(), []string{source, t.TempDir()}, &stdout, &stderr); exitCode != 1 {
		t.Fatalf("unexpected exit code: %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "unsupported document format") {
		t.Fatalf("unexpected error: %s", stderr.String())
	}
}

func TestRunUsesConfiguredLibreOffice(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	outputDirectory := t.TempDir()
	source := filepath.Join("..", "..", "test", "documents", "samplefile.pdf")
	if exitCode := Run(context.Background(), []string{"--libreoffice", "/custom/libreoffice", source, outputDirectory}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("unexpected exit code %d: %s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "samplefile-page-") {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}
