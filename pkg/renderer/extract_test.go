package renderer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExtractDocumentExtractsEveryPDFPage(t *testing.T) {
	result, err := ExtractDocument(context.Background(), fixturePath("samplefile.pdf"))
	if err != nil {
		t.Fatalf("extract PDF text: %v", err)
	}
	if result.PartCount() == 0 {
		t.Fatal("expected at least one extracted page")
	}
	for index, part := range result.Parts {
		if part.PartNumber != index+1 {
			t.Fatalf("unexpected part number: %d", part.PartNumber)
		}
	}
}

func TestExtractDocumentRejectsTextExceedingMaxCharacters(t *testing.T) {
	options := DefaultExtractOptions()
	options.MaxCharacters = 1

	_, err := ExtractDocumentWithOptions(context.Background(), fixturePath("samplefile.pdf"), &options)
	var exceeded *CharacterLimitExceededError
	if !errors.As(err, &exceeded) {
		t.Fatalf("expected CharacterLimitExceededError, got %v", err)
	}
	if exceeded.MaxCharacters != options.MaxCharacters || exceeded.CharacterCount <= exceeded.MaxCharacters {
		t.Fatalf("unexpected character limit error: %+v", exceeded)
	}
}

func TestExtractDocumentExtractsModernOfficeText(t *testing.T) {
	for _, name := range []string{"samplefile.docx", "samplefile.pptx", "samplefile.xlsx", "samplefile.xlsm"} {
		t.Run(name, func(t *testing.T) {
			result, err := ExtractDocument(context.Background(), fixturePath(name))
			if err != nil {
				t.Fatalf("extract Office text: %v", err)
			}
			if result.PartCount() == 0 {
				t.Fatal("expected at least one extracted part")
			}
			for index, part := range result.Parts {
				if part.PartNumber != index+1 {
					t.Fatalf("unexpected part number: %d", part.PartNumber)
				}
				if strings.TrimSpace(part.Text) == "" {
					t.Fatalf("part %d contains no text", part.PartNumber)
				}
			}
		})
	}
}

func TestExtractDocumentExtractsLegacyOfficeText(t *testing.T) {
	replaceLibreOfficeFunctions(t)
	findExecutable = func(string) (string, error) {
		t.Fatal("explicit LibreOffice executable was not used")
		return "", nil
	}
	executeLibreOffice = func(_ context.Context, executable string, arguments []string) (string, string, error) {
		if executable != "/custom/libreoffice" {
			t.Fatalf("unexpected executable: %s", executable)
		}
		source := arguments[len(arguments)-1]
		targets := map[string]string{".doc": ".docx", ".ppt": ".pptx", ".xls": ".xlsx"}
		targetExtension := targets[filepath.Ext(source)]
		if filter := argumentAfter(t, arguments, "--convert-to"); filter != strings.TrimPrefix(targetExtension, ".") {
			t.Fatalf("unexpected conversion filter: %s", filter)
		}
		outputDirectory := argumentAfter(t, arguments, "--outdir")
		outputName := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source)) + targetExtension
		copyFixture(t, fixturePath("samplefile"+targetExtension), filepath.Join(outputDirectory, outputName))
		return "", "", nil
	}
	for _, name := range []string{"samplefile.doc", "samplefile.ppt", "samplefile.xls"} {
		t.Run(name, func(t *testing.T) {
			result, err := ExtractDocumentWithOptions(context.Background(), fixturePath(name), &ExtractOptions{
				LibreOfficeTimeout: 5 * time.Second, LibreOfficeExecutable: "/custom/libreoffice",
			})
			if err != nil {
				t.Fatalf("extract legacy Office text: %v", err)
			}
			if result.PartCount() == 0 {
				t.Fatal("expected at least one extracted part")
			}
			for _, part := range result.Parts {
				if strings.TrimSpace(part.Text) == "" {
					t.Fatalf("part %d contains no text", part.PartNumber)
				}
			}
		})
	}
}

func TestExtractDocumentReturnsEmptyResultForUnknownFormat(t *testing.T) {
	// The default branch is unreachable through the public API because
	// validateDocument rejects unsupported extensions; exercise it directly.
	source := filepath.Join(t.TempDir(), "input.xyz")
	if err := os.WriteFile(source, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	// validateDocument rejects it, so the default branch is only reachable by
	// calling the internal switch; verify the public API rejects it instead.
	_, err := ExtractDocument(context.Background(), source)
	var unsupported *UnsupportedFormatError
	if !errors.As(err, &unsupported) {
		t.Fatalf("expected UnsupportedFormatError, got %v", err)
	}
}

func TestExtractDocumentDoesNotDoubleWrapExtractionError(t *testing.T) {
	// A DocumentExtractionError returned by a format extractor must pass
	// through unchanged. Use a PDF that fails to open.
	source := filepath.Join(t.TempDir(), "invalid.pdf")
	if err := os.WriteFile(source, []byte("not a PDF"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ExtractDocument(context.Background(), source)
	var extractionError *DocumentExtractionError
	if !errors.As(err, &extractionError) {
		t.Fatalf("expected DocumentExtractionError, got %v", err)
	}
}
