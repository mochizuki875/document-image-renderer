package renderer

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConvertOfficeUsesIsolatedProfile(t *testing.T) {
	source := filepath.Join(t.TempDir(), "input.docx")
	if err := os.WriteFile(source, []byte("placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	replaceLibreOfficeFunctions(t)
	findExecutable = func(string) (string, error) { return "/usr/bin/libreoffice", nil }
	executeLibreOffice = func(_ context.Context, executable string, arguments []string) (string, string, error) {
		if executable != "/usr/bin/libreoffice" {
			t.Fatalf("unexpected executable: %s", executable)
		}
		profileArgument := argumentWithPrefix(arguments, "-env:UserInstallation=")
		profileURL, err := url.Parse(strings.TrimPrefix(profileArgument, "-env:UserInstallation="))
		if err != nil {
			t.Fatal(err)
		}
		profile, err := os.ReadFile(filepath.Join(profileURL.Path, "user", "registrymodifications.xcu"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(profile), `oor:name="MacroSecurityLevel"`) || !strings.Contains(string(profile), "<value>3</value>") {
			t.Fatalf("macro security is not configured: %s", profile)
		}
		outputDirectory := argumentAfter(t, arguments, "--outdir")
		copyFixture(t, fixturePath("samplefile.pdf"), filepath.Join(outputDirectory, "input.pdf"))
		return "converted", "", nil
	}

	result, err := RenderDocument(context.Background(), source, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("render Office document: %v", err)
	}
	if result.PageCount() == 0 {
		t.Fatal("expected at least one rendered page")
	}
}

func TestPageCountConvertsOfficeDocument(t *testing.T) {
	source := filepath.Join(t.TempDir(), "input.docx")
	if err := os.WriteFile(source, []byte("placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	replaceLibreOfficeFunctions(t)
	findExecutable = func(string) (string, error) { return "/usr/bin/libreoffice", nil }
	executeLibreOffice = func(_ context.Context, executable string, arguments []string) (string, string, error) {
		if executable != "/usr/bin/libreoffice" {
			t.Fatalf("unexpected executable: %s", executable)
		}
		outputDirectory := argumentAfter(t, arguments, "--outdir")
		copyFixture(t, fixturePath("samplefile.pdf"), filepath.Join(outputDirectory, "input.pdf"))
		return "", "", nil
	}

	pageCount, err := PageCount(context.Background(), source)
	if err != nil {
		t.Fatalf("count Office document pages: %v", err)
	}
	if pageCount == 0 {
		t.Fatal("expected at least one page")
	}
}

func TestConvertOfficeAllowsUnlimitedTimeout(t *testing.T) {
	source := filepath.Join(t.TempDir(), "input.docx")
	if err := os.WriteFile(source, []byte("placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	replaceLibreOfficeFunctions(t)
	findExecutable = func(string) (string, error) { return "/usr/bin/libreoffice", nil }
	executeLibreOffice = func(ctx context.Context, _ string, arguments []string) (string, string, error) {
		if _, hasDeadline := ctx.Deadline(); hasDeadline {
			t.Fatal("unlimited timeout unexpectedly set a deadline")
		}
		outputDirectory := argumentAfter(t, arguments, "--outdir")
		copyFixture(t, fixturePath("samplefile.pdf"), filepath.Join(outputDirectory, "input.pdf"))
		return "", "", nil
	}

	options := DefaultRenderOptions()
	options.LibreOfficeTimeout = 0
	result, err := RenderDocument(context.Background(), source, t.TempDir(), &options)
	if err != nil {
		t.Fatalf("render Office document: %v", err)
	}
	if result.PageCount() == 0 {
		t.Fatal("expected at least one rendered page")
	}
}

func TestExcelFormatsUseSinglePageFilter(t *testing.T) {
	for _, extension := range []string{".xls", ".xlsx", ".xlsm"} {
		t.Run(extension, func(t *testing.T) {
			if filter := pdfConversionFilter(extension); filter != calcSinglePageFilter {
				t.Fatalf("filter = %q, want SinglePageSheets filter", filter)
			}
		})
	}
}

func TestOfficeDocumentRequiresLibreOffice(t *testing.T) {
	source := filepath.Join(t.TempDir(), "input.docx")
	if err := os.WriteFile(source, []byte("placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	replaceLibreOfficeFunctions(t)
	findExecutable = func(string) (string, error) { return "", errors.New("not found") }

	_, err := RenderDocument(context.Background(), source, t.TempDir(), nil)
	var dependencyError *DependencyNotFoundError
	if !errors.As(err, &dependencyError) {
		t.Fatalf("expected DependencyNotFoundError, got %v", err)
	}
}

func TestConversionErrorRetainsDiagnostics(t *testing.T) {
	source := filepath.Join(t.TempDir(), "input.ppt")
	if err := os.WriteFile(source, []byte("placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	replaceLibreOfficeFunctions(t)
	findExecutable = func(string) (string, error) { return "libreoffice", nil }
	executeLibreOffice = func(context.Context, string, []string) (string, string, error) {
		return "output", "failure", errors.New("exit status 1")
	}

	_, err := RenderDocument(context.Background(), source, t.TempDir(), nil)
	var conversionError *DocumentConversionError
	if !errors.As(err, &conversionError) {
		t.Fatalf("expected DocumentConversionError, got %v", err)
	}
	if conversionError.Stdout != "output" || conversionError.Stderr != "failure" {
		t.Fatalf("diagnostics were not retained: %+v", conversionError)
	}
}

func replaceLibreOfficeFunctions(t *testing.T) {
	t.Helper()
	originalFind := findExecutable
	originalExecute := executeLibreOffice
	t.Cleanup(func() {
		findExecutable = originalFind
		executeLibreOffice = originalExecute
	})
}

func argumentWithPrefix(arguments []string, prefix string) string {
	for _, argument := range arguments {
		if strings.HasPrefix(argument, prefix) {
			return argument
		}
	}
	return ""
}

func argumentAfter(t *testing.T, arguments []string, target string) string {
	t.Helper()
	for index, argument := range arguments {
		if argument == target && index+1 < len(arguments) {
			return arguments[index+1]
		}
	}
	t.Fatalf("argument %s not found in %v", target, arguments)
	return ""
}

func copyFixture(t *testing.T, source, destination string) {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
