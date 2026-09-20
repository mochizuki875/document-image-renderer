package renderer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const calcSinglePageFilter = `pdf:calc_pdf_Export:{"SinglePageSheets":{"type":"boolean","value":"true"}}`

const libreOfficeProfile = `<?xml version="1.0" encoding="UTF-8"?>
<oor:items xmlns:oor="http://openoffice.org/2001/registry">
    <item oor:path="/org.openoffice.Office.Common/Security/Scripting">
        <prop oor:name="MacroSecurityLevel" oor:op="fuse"><value>3</value></prop>
    </item>
</oor:items>
`

var findExecutable = exec.LookPath

var executeLibreOffice = func(ctx context.Context, executable string, arguments []string) (string, string, error) {
	command := exec.CommandContext(ctx, executable, arguments...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}

type libreOfficeConfig struct {
	timeout    time.Duration
	executable string
}

func convertOfficeToPDF(
	ctx context.Context,
	source string,
	extension string,
	options RenderOptions,
) (string, func(), error) {
	return convertOffice(ctx, source, extension, officeConversion{
		targetExtension: ".pdf", filter: pdfConversionFilter(extension), prepareSource: true,
	}, libreOfficeConfig{timeout: options.LibreOfficeTimeout, executable: options.LibreOfficeExecutable})
}

func pdfConversionFilter(extension string) string {
	switch extension {
	case ".xls", ".xlsx", ".xlsm":
		return calcSinglePageFilter
	default:
		return "pdf"
	}
}

func convertLegacyOfficeToOOXML(
	ctx context.Context,
	source string,
	extension string,
	config libreOfficeConfig,
) (string, func(), error) {
	targets := map[string]string{
		".doc": ".docx",
		".ppt": ".pptx",
		".xls": ".xlsx",
	}
	targetExtension, supported := targets[extension]
	if !supported {
		return "", func() {}, fmt.Errorf("unsupported legacy Office format: %s", extension)
	}
	return convertOffice(ctx, source, extension, officeConversion{
		targetExtension: targetExtension, filter: strings.TrimPrefix(targetExtension, "."),
	}, config)
}

type officeConversion struct {
	targetExtension string
	filter          string
	prepareSource   bool
}

func convertOffice(
	ctx context.Context,
	source string,
	extension string,
	conversion officeConversion,
	config libreOfficeConfig,
) (string, func(), error) {
	executable := config.executable
	if executable == "" {
		for _, candidate := range []string{"libreoffice", "soffice"} {
			found, err := findExecutable(candidate)
			if err == nil {
				executable = found
				break
			}
		}
	}
	if executable == "" {
		return "", func() {}, &DependencyNotFoundError{
			Dependency: "LibreOffice",
			Operation:  fmt.Sprintf("convert %s documents", extension),
		}
	}

	workingDirectory, err := os.MkdirTemp("", "document-image-renderer-")
	if err != nil {
		return "", func() {}, &DocumentConversionError{Path: source, Err: err}
	}
	cleanup := func() { _ = os.RemoveAll(workingDirectory) }
	fail := func(err error, stdout, stderr string) (string, func(), error) {
		cleanup()
		return "", func() {}, &DocumentConversionError{
			Path: source, Stdout: stdout, Stderr: stderr, Err: err,
		}
	}

	outputDirectory := filepath.Join(workingDirectory, "output")
	profileDirectory := filepath.Join(workingDirectory, "profile")
	if err := os.MkdirAll(outputDirectory, 0o700); err != nil {
		return fail(err, "", "")
	}
	if err := configureLibreOfficeProfile(profileDirectory); err != nil {
		return fail(err, "", "")
	}

	conversionSource := source
	if conversion.prepareSource {
		conversionSource = prepareOfficeSource(source, extension, workingDirectory)
	}
	profileURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(profileDirectory)}).String()
	arguments := []string{
		"--headless",
		"--nologo",
		"--nodefault",
		"--nolockcheck",
		"--nofirststartwizard",
		"-env:UserInstallation=" + profileURL,
		"--convert-to",
		conversion.filter,
		"--outdir",
		outputDirectory,
		conversionSource,
	}

	timeoutContext, cancel := context.WithTimeout(ctx, config.timeout)
	defer cancel()
	stdout, stderr, commandErr := executeLibreOffice(timeoutContext, executable, arguments)
	if timeoutErr := timeoutContext.Err(); errors.Is(timeoutErr, context.DeadlineExceeded) {
		return fail(fmt.Errorf("LibreOffice timed out: %w", timeoutErr), stdout, stderr)
	}
	if commandErr != nil {
		return fail(commandErr, stdout, stderr)
	}

	outputName := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source)) + conversion.targetExtension
	outputPath := filepath.Join(outputDirectory, outputName)
	if info, err := os.Stat(outputPath); err != nil || !info.Mode().IsRegular() {
		return fail(fmt.Errorf("LibreOffice did not produce %s", outputName), stdout, stderr)
	}
	return outputPath, cleanup, nil
}

func configureLibreOfficeProfile(profileDirectory string) error {
	userDirectory := filepath.Join(profileDirectory, "user")
	if err := os.MkdirAll(userDirectory, 0o700); err != nil {
		return err
	}
	return os.WriteFile(
		filepath.Join(userDirectory, "registrymodifications.xcu"),
		[]byte(libreOfficeProfile),
		0o600,
	)
}
