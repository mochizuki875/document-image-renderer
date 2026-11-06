package renderer

import "testing"

func TestDefaultExtractOptionsAreValid(t *testing.T) {
	options := DefaultExtractOptions()
	if err := options.Validate(); err != nil {
		t.Fatalf("default options must be valid: %v", err)
	}
	if options.LibreOfficeTimeout <= 0 {
		t.Fatalf("unexpected defaults: %+v", options)
	}
	if options.MaxPDFBytes == 0 {
		t.Fatal("default PDF byte limit must be finite")
	}
}

func TestExtractOptionsRejectNegativeMaxCharacters(t *testing.T) {
	options := DefaultExtractOptions()
	options.MaxCharacters = -1
	if err := options.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestExtractOptionsRejectNegativeMaxOOXMLMembers(t *testing.T) {
	options := DefaultExtractOptions()
	options.MaxOOXMLMembers = -1
	if err := options.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestExtractOptionsRejectNegativeLibreOfficeTimeout(t *testing.T) {
	options := DefaultExtractOptions()
	options.LibreOfficeTimeout = -1
	if err := options.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestExtractOptionsAllowUnlimitedLibreOfficeTimeout(t *testing.T) {
	options := DefaultExtractOptions()
	options.LibreOfficeTimeout = 0
	if err := options.Validate(); err != nil {
		t.Fatalf("extract options must allow an unlimited timeout: %v", err)
	}
}
