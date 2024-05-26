// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package reader provides utils around io.Reader.
package reader

import (
	"bytes"
	"errors"
	"io"
	"slices"
)

const stringReaderBufferSize = 1024 * 10

// Index returns the index of the first occurrence of toFind in r.
// It returns -1 if toFind is not present in r.
// It returns an error if reading from r other than io.EOF returns an error.
func Index(r io.Reader, toFind string) (int, error) {
	bufLen := stringReaderBufferSize
	if len(toFind) > bufLen {
		bufLen = len(toFind)
	}
	buf := make([]byte, bufLen)
	bytesToFind := []byte(toFind)
	total := 0
	suffix := make([]byte, 0, len(bytesToFind))
	done := false
	for !done {
		n, err := r.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				done = true
			} else {
				return -1, err
			}
		}
		sufLen := len(suffix)
		// you only need to check if the matching term is overlapping a buffer boundary if:
		//
		// - part of the matching term in the previous buffer (sufLen > 0)
		// - the contents of the current buffer are long enough to contain the portion of the
		//   matching term that wasn't at the end of the previous buffer (n >= len(byteToFind) - sufLen).
		if sufLen > 0 && n >= len(bytesToFind)-sufLen {
			suffix = append(suffix, buf[:len(bytesToFind)-sufLen]...)
			if bytes.Equal(suffix, bytesToFind) {
				return total - sufLen, nil
			}
		}
		if offset := bytes.Index(buf[:n], bytesToFind); offset != -1 {
			return total + offset, nil
		}
		total += n
		if !done {
			suffix = suffix[:0]
			potential := findPrefixAtEnd(buf[:n], bytesToFind)
			if potential != nil {
				suffix = append(suffix, potential...)
			}
		}
	}
	return -1, nil
}

func findPrefixAtEnd(buf []byte, toFind []byte) []byte {
	start := len(toFind) - 1
	if start >= len(buf) {
		start = len(buf) - 1
	}
	for i := start; i >= 0; i-- {
		curMatch := toFind[:i+1]
		potential := buf[len(buf)-i-1:]
		if slices.Equal(potential, curMatch) {
			return potential
		}
	}

	return nil
}
