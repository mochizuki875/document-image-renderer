package renderer

import (
	"fmt"
	"time"
)

// ExtractOptions controls extraction limits and dependencies.
// The zero value is not usable; call DefaultExtractOptions and override fields.
type ExtractOptions struct {
	MaxCharacters         int
	MaxPDFBytes           uint64
	MaxOOXMLMembers       int
	MaxOOXMLMemberBytes   uint64
	MaxOOXMLTotalBytes    uint64
	LibreOfficeTimeout    time.Duration
	LibreOfficeExecutable string
}

// DefaultExtractOptions returns the default extraction configuration.
// MaxCharacters is zero, meaning no character limit.
func DefaultExtractOptions() ExtractOptions {
	return ExtractOptions{
		MaxPDFBytes:         128 << 20,
		MaxOOXMLMembers:     10_000,
		MaxOOXMLMemberBytes: 256 << 20,
		MaxOOXMLTotalBytes:  1 << 30,
		LibreOfficeTimeout:  120 * time.Second,
	}
}

// Validate checks whether all extraction option values are valid.
func (options ExtractOptions) Validate() error {
	if options.MaxCharacters < 0 {
		return fmt.Errorf("max characters must not be negative")
	}
	if options.MaxOOXMLMembers < 0 {
		return fmt.Errorf("max OOXML members must not be negative")
	}
	if options.LibreOfficeTimeout < 0 {
		return fmt.Errorf("libreoffice timeout must not be negative")
	}
	return nil
}
