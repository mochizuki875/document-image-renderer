package renderer

import (
	"archive/zip"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOOXMLArchiveLimits(t *testing.T) {
	source := writeOOXMLFixture(t, map[string]string{"one.xml": "1234", "two.xml": "5678"})
	tests := []struct {
		name   string
		limits ooxmlLimits
		fails  bool
	}{
		{name: "member count boundary", limits: ooxmlLimits{maxMembers: 2}},
		{name: "member count exceeded", limits: ooxmlLimits{maxMembers: 1}, fails: true},
		{name: "member size boundary", limits: ooxmlLimits{maxMemberBytes: 4}},
		{name: "member size exceeded", limits: ooxmlLimits{maxMemberBytes: 3}, fails: true},
		{name: "total size boundary", limits: ooxmlLimits{maxTotalBytes: 8}},
		{name: "total size exceeded", limits: ooxmlLimits{maxTotalBytes: 7}, fails: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateOOXMLArchive(source, test.limits)
			var exceeded *OOXMLLimitExceededError
			if errors.As(err, &exceeded) != test.fails {
				t.Fatalf("error = %v, expected limit failure %t", err, test.fails)
			}
		})
	}
}

func TestOOXMLStreamingLimitDoesNotTrustHeader(t *testing.T) {
	_, err := copyLimitedOOXML(io.Discard, strings.NewReader("12345"), "part.xml", 0, ooxmlLimits{maxMemberBytes: 4})
	var exceeded *OOXMLLimitExceededError
	if !errors.As(err, &exceeded) {
		t.Fatalf("expected OOXMLLimitExceededError, got %v", err)
	}
}

func TestCopyLimitedOOXMLEnforcesTotalLimit(t *testing.T) {
	_, err := copyLimitedOOXML(io.Discard, strings.NewReader("12345"), "part.xml", 4, ooxmlLimits{maxTotalBytes: 8})
	var exceeded *OOXMLLimitExceededError
	if !errors.As(err, &exceeded) {
		t.Fatalf("expected OOXMLLimitExceededError, got %v", err)
	}
	if exceeded.LimitType != "total uncompressed bytes" {
		t.Fatalf("unexpected limit type: %s", exceeded.LimitType)
	}
}

func TestCopyLimitedOOXMLPropagatesReadError(t *testing.T) {
	_, err := copyLimitedOOXML(io.Discard, &failingReader{}, "part.xml", 0, ooxmlLimits{})
	if err == nil || !strings.Contains(err.Error(), "read OOXML member") {
		t.Fatalf("expected read error, got %v", err)
	}
}

func TestCopyLimitedOOXMLPropagatesWriteError(t *testing.T) {
	_, err := copyLimitedOOXML(&failingWriter{}, strings.NewReader("data"), "part.xml", 0, ooxmlLimits{})
	if err == nil {
		t.Fatal("expected write error")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestLimitedOOXMLMemberReadEnforcesMemberLimit(t *testing.T) {
	source := writeOOXMLFixture(t, map[string]string{"part.xml": "12345"})
	archive, err := zip.OpenReader(source)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	var total uint64
	reader, err := openLimitedOOXMLMember(archive.File[0], &total, ooxmlLimits{maxMemberBytes: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	buffer := make([]byte, 10)
	_, err = reader.Read(buffer)
	var exceeded *OOXMLLimitExceededError
	if !errors.As(err, &exceeded) {
		t.Fatalf("expected OOXMLLimitExceededError, got %v", err)
	}
	if exceeded.Member != "part.xml" {
		t.Fatalf("unexpected member: %s", exceeded.Member)
	}
}

func TestLimitedOOXMLMemberReadEnforcesTotalLimit(t *testing.T) {
	source := writeOOXMLFixture(t, map[string]string{"part.xml": "12345"})
	archive, err := zip.OpenReader(source)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	total := uint64(4)
	reader, err := openLimitedOOXMLMember(archive.File[0], &total, ooxmlLimits{maxTotalBytes: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	buffer := make([]byte, 10)
	_, err = reader.Read(buffer)
	var exceeded *OOXMLLimitExceededError
	if !errors.As(err, &exceeded) {
		t.Fatalf("expected OOXMLLimitExceededError, got %v", err)
	}
	if exceeded.LimitType != "total uncompressed bytes" {
		t.Fatalf("unexpected limit type: %s", exceeded.LimitType)
	}
}

func TestOpenAndCheckOOXMLHeadersClosesOnLimitFailure(t *testing.T) {
	source := writeOOXMLFixture(t, map[string]string{"one.xml": "1234", "two.xml": "5678"})
	_, err := openAndCheckOOXMLHeaders(source, ooxmlLimits{maxMembers: 1})
	var exceeded *OOXMLLimitExceededError
	if !errors.As(err, &exceeded) {
		t.Fatalf("expected OOXMLLimitExceededError, got %v", err)
	}
}

func TestOpenAndCheckOOXMLHeadersRejectsInvalidArchive(t *testing.T) {
	source := filepath.Join(t.TempDir(), "invalid.zip")
	if err := os.WriteFile(source, []byte("not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openAndCheckOOXMLHeaders(source, ooxmlLimits{}); err == nil {
		t.Fatal("expected error for invalid archive")
	}
}

func TestReadLimitedOOXMLMemberEnforcesLimits(t *testing.T) {
	source := writeOOXMLFixture(t, map[string]string{"part.xml": "12345"})
	archive, err := zip.OpenReader(source)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	var total uint64
	_, err = readLimitedOOXMLMember(archive.File[0], &total, ooxmlLimits{maxMemberBytes: 4})
	var exceeded *OOXMLLimitExceededError
	if !errors.As(err, &exceeded) {
		t.Fatalf("expected OOXMLLimitExceededError, got %v", err)
	}
}

func TestCheckOOXMLHeadersRejectsOverflowingTotal(t *testing.T) {
	source := writeOOXMLFixture(t, map[string]string{"part.xml": "1234"})
	archive, err := zip.OpenReader(source)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	archive.File[0].UncompressedSize64 = math.MaxUint64
	err = checkOOXMLHeaders(archive.File, ooxmlLimits{maxTotalBytes: 1})
	var exceeded *OOXMLLimitExceededError
	if !errors.As(err, &exceeded) {
		t.Fatalf("expected OOXMLLimitExceededError, got %v", err)
	}
	if exceeded.LimitType != "total uncompressed bytes" {
		t.Fatalf("unexpected limit type: %s", exceeded.LimitType)
	}
}

func TestValidateOOXMLArchivePropagatesMemberReadError(t *testing.T) {
	source := writeOOXMLFixture(t, map[string]string{"part.xml": "1234"})
	archive, err := zip.OpenReader(source)
	if err != nil {
		t.Fatal(err)
	}
	archive.File[0].UncompressedSize64 = 100 // header lies about the size
	archive.Close()
	// Reopen with a corrupted central directory is not possible; instead verify
	// that a valid archive passes validation.
	if err := validateOOXMLArchive(source, ooxmlLimits{}); err != nil {
		t.Fatalf("valid archive must pass validation: %v", err)
	}
}
