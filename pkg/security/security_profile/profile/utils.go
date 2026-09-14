// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package profile holds profile related files
package profile

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// maxDecompressedSize is the maximum number of bytes unzip will write to disk when
// decompressing a security-profile gzip file. It guards against decompression bombs;
// exceeding it is an error, not a silent truncation.
const maxDecompressedSize = 2 << 30 // 2 GiB

func unzip(inputFile string, ext string) (string, error) {
	// uncompress the file first
	f, err := os.Open(inputFile)
	if err != nil {
		return "", fmt.Errorf("couldn't open input file: %w", err)
	}
	defer f.Close()

	seclog.Infof("unzipping %s", inputFile)
	gzipReader, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("couldn't create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	outputFile, err := os.Create(strings.TrimSuffix(inputFile, ext))
	if err != nil {
		return "", fmt.Errorf("couldn't create gzip output file: %w", err)
	}
	defer outputFile.Close()

	n, err := io.CopyN(outputFile, gzipReader, maxDecompressedSize+1)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("couldn't unzip %s: %w", inputFile, err)
	}
	if n > maxDecompressedSize {
		return "", fmt.Errorf("couldn't unzip %s: decompressed size exceeds maximum allowed size of %d bytes", inputFile, maxDecompressedSize)
	}

	if err = outputFile.Close(); err != nil {
		return "", fmt.Errorf("could not close file [%s]: %w", outputFile.Name(), err)
	}

	return strings.TrimSuffix(inputFile, ext), nil
}
