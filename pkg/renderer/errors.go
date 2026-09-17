package renderer

import "fmt"

// UnsupportedFormatError reports an unsupported input extension.
type UnsupportedFormatError struct {
	Extension string
	Path      string
}

func (err *UnsupportedFormatError) Error() string {
	extension := err.Extension
	if extension == "" {
		extension = "<none>"
	}
	return fmt.Sprintf("unsupported document format %q: %s", extension, err.Path)
}

// DependencyNotFoundError reports a missing external dependency.
type DependencyNotFoundError struct {
	Dependency string
	Operation  string
}

func (err *DependencyNotFoundError) Error() string {
	return fmt.Sprintf("%s is required to %s", err.Dependency, err.Operation)
}

// DocumentConversionError reports an Office-to-PDF conversion failure.
type DocumentConversionError struct {
	Path   string
	Stdout string
	Stderr string
	Err    error
}

func (err *DocumentConversionError) Error() string {
	return fmt.Sprintf("could not convert document to PDF %q: %v", err.Path, err.Err)
}

func (err *DocumentConversionError) Unwrap() error { return err.Err }

// DocumentRenderError reports a PDF rasterization failure.
type DocumentRenderError struct {
	Path string
	Err  error
}

func (err *DocumentRenderError) Error() string {
	return fmt.Sprintf("could not render PDF %q: %v", err.Path, err.Err)
}

func (err *DocumentRenderError) Unwrap() error { return err.Err }
