// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package replay

import (
	"fmt"
	"io"

	"github.com/h2non/filetype"
	"github.com/h2non/filetype/matchers"
)

var (
	datadogType = filetype.NewType("dog", "datadog/capture")
	// DATADOG0F1FF0000 in HEX (D474D060F1FF0000); (F0 | datadogFileVersion) for different file versions support
	// 00 to terminate header
	datadogHeader  = []byte{0xD4, 0x74, 0xD0, 0x60, 0xF0, 0xFF, 0x00, 0x00}
	ErrHeaderWrite = fmt.Errorf("capture file header could not be fully written to buffer")
)

const (
	// Version 3+ adds support for nanosecond cadence.
	// Version 2+ adds support for storing state.
	datadogFileVersion uint8 = 3

	versionIndex    = 4
	minStateVersion = 2
	minNanoVersion  = 3
)

func init() {
	// Register the new matcher and its type
	filetype.AddMatcher(datadogType, datadogMatcher)
	filetype.AddMatcher(matchers.TypeZstd, matchers.Zst)
}

func datadogMatcher(buf []byte) bool {
	if len(buf) < len(datadogHeader) {
		return false
	}

	for i := 0; i < len(datadogHeader); i++ {
		if i == versionIndex {
			if buf[i]&datadogHeader[i] != datadogHeader[i] {
				return false
			}
		} else if buf[i] != datadogHeader[i] {
			return false
		}
	}

	return true
}

func fileVersion(buf []byte) (int, error) {

	if !datadogMatcher(buf) {
		return -1, fmt.Errorf("Cannot verify file version bad buffer or invalid file")
	}

	ver := int(0xF0 ^ buf[4])
	if ver > int(datadogFileVersion) {
		return -1, fmt.Errorf("Unsupported file version")
	}
	return ver, nil
}

// WriteHeader writes the datadog header to the Writer argument to conform to the .dog file format.
func WriteHeader(w io.Writer) error {
	hdr := make([]byte, len(datadogHeader))
	copy(hdr, datadogHeader)
	hdr[versionIndex] |= datadogFileVersion

	//Write header
	n, err := w.Write(hdr)

	if err != nil {
		return err
	}

	if n < len(datadogHeader) {
		return ErrHeaderWrite
	}

	return nil
}
