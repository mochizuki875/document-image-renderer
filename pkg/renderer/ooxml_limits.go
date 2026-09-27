package renderer

import (
	"archive/zip"
	"fmt"
	"io"
	"math"
)

type ooxmlLimits struct {
	maxMembers     int
	maxMemberBytes uint64
	maxTotalBytes  uint64
}

func renderOOXMLLimits(options RenderOptions) ooxmlLimits {
	return ooxmlLimits{options.MaxOOXMLMembers, options.MaxOOXMLMemberBytes, options.MaxOOXMLTotalBytes}
}

func extractOOXMLLimits(options ExtractOptions) ooxmlLimits {
	return ooxmlLimits{options.MaxOOXMLMembers, options.MaxOOXMLMemberBytes, options.MaxOOXMLTotalBytes}
}

func checkOOXMLHeaders(files []*zip.File, limits ooxmlLimits) error {
	if limits.maxMembers > 0 && len(files) > limits.maxMembers {
		return &OOXMLLimitExceededError{LimitType: "members", Actual: uint64(len(files)), Limit: uint64(limits.maxMembers)}
	}
	var total uint64
	for _, file := range files {
		size := file.UncompressedSize64
		if limits.maxMemberBytes > 0 && size > limits.maxMemberBytes {
			return &OOXMLLimitExceededError{LimitType: "uncompressed bytes", Member: file.Name, Actual: size, Limit: limits.maxMemberBytes}
		}
		if size > math.MaxUint64-total {
			return &OOXMLLimitExceededError{LimitType: "total uncompressed bytes", Actual: math.MaxUint64, Limit: limits.maxTotalBytes}
		}
		if limits.maxTotalBytes > 0 && (total > limits.maxTotalBytes || size > limits.maxTotalBytes-total) {
			return &OOXMLLimitExceededError{LimitType: "total uncompressed bytes", Actual: total + size, Limit: limits.maxTotalBytes}
		}
		total += size
	}
	return nil
}

func validateOOXMLArchive(source string, limits ooxmlLimits) error {
	archive, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer archive.Close()
	if err := checkOOXMLHeaders(archive.File, limits); err != nil {
		return err
	}
	var total uint64
	for _, file := range archive.File {
		input, err := file.Open()
		if err != nil {
			return err
		}
		memberBytes, copyErr := copyLimitedOOXML(io.Discard, input, file.Name, total, limits)
		closeErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		total += memberBytes
	}
	return nil
}

func openAndCheckOOXMLHeaders(source string, limits ooxmlLimits) (*zip.ReadCloser, error) {
	archive, err := zip.OpenReader(source)
	if err != nil {
		return nil, err
	}
	if err := checkOOXMLHeaders(archive.File, limits); err != nil {
		_ = archive.Close()
		return nil, err
	}
	return archive, nil
}

func readLimitedOOXMLMember(file *zip.File, total *uint64, limits ooxmlLimits) ([]byte, error) {
	input, err := openLimitedOOXMLMember(file, total, limits)
	if err != nil {
		return nil, err
	}
	defer input.Close()
	return io.ReadAll(input)
}

type limitedOOXMLMember struct {
	io.ReadCloser
	name        string
	total       *uint64
	memberBytes uint64
	limits      ooxmlLimits
}

func openLimitedOOXMLMember(file *zip.File, total *uint64, limits ooxmlLimits) (*limitedOOXMLMember, error) {
	input, err := file.Open()
	if err != nil {
		return nil, err
	}
	return &limitedOOXMLMember{ReadCloser: input, name: file.Name, total: total, limits: limits}, nil
}

func (reader *limitedOOXMLMember) Read(buffer []byte) (int, error) {
	count, err := reader.ReadCloser.Read(buffer)
	if count == 0 {
		return count, err
	}
	chunk := uint64(count)
	if reader.limits.maxMemberBytes > 0 && (reader.memberBytes > reader.limits.maxMemberBytes || chunk > reader.limits.maxMemberBytes-reader.memberBytes) {
		return 0, &OOXMLLimitExceededError{LimitType: "uncompressed bytes", Member: reader.name, Actual: reader.memberBytes + chunk, Limit: reader.limits.maxMemberBytes}
	}
	if reader.limits.maxTotalBytes > 0 && (*reader.total > reader.limits.maxTotalBytes || chunk > reader.limits.maxTotalBytes-*reader.total) {
		return 0, &OOXMLLimitExceededError{LimitType: "total uncompressed bytes", Actual: *reader.total + chunk, Limit: reader.limits.maxTotalBytes}
	}
	reader.memberBytes += chunk
	*reader.total += chunk
	return count, err
}

func copyLimitedOOXML(destination io.Writer, source io.Reader, member string, total uint64, limits ooxmlLimits) (uint64, error) {
	buffer := make([]byte, 32*1024)
	var memberBytes uint64
	for {
		count, readErr := source.Read(buffer)
		if count > 0 {
			chunk := uint64(count)
			if limits.maxMemberBytes > 0 && (memberBytes > limits.maxMemberBytes || chunk > limits.maxMemberBytes-memberBytes) {
				return memberBytes + chunk, &OOXMLLimitExceededError{LimitType: "uncompressed bytes", Member: member, Actual: memberBytes + chunk, Limit: limits.maxMemberBytes}
			}
			if limits.maxTotalBytes > 0 && (total > limits.maxTotalBytes || memberBytes > limits.maxTotalBytes-total || chunk > limits.maxTotalBytes-total-memberBytes) {
				return memberBytes + chunk, &OOXMLLimitExceededError{LimitType: "total uncompressed bytes", Actual: total + memberBytes + chunk, Limit: limits.maxTotalBytes}
			}
			if _, err := destination.Write(buffer[:count]); err != nil {
				return memberBytes, err
			}
			memberBytes += chunk
		}
		if readErr == io.EOF {
			return memberBytes, nil
		}
		if readErr != nil {
			return memberBytes, fmt.Errorf("read OOXML member %q: %w", member, readErr)
		}
	}
}
