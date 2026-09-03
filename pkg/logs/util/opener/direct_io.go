// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Block-alignment rules for Linux O_DIRECT reads. Unexported helpers back the
// bounded fingerprint entry points in open.go.

package opener

import (
	"errors"
	"io"
	"os"
	"unsafe"
)

const directIOAlignment = 4096

// readDirectFingerprintRange opens path with O_DIRECT and returns up to the
// first count bytes.
func readDirectFingerprintRange(path string, count int) ([]byte, error) {
	if count < 0 {
		return nil, os.ErrInvalid
	}
	if count == 0 {
		return []byte{}, nil
	}

	file, err := openDirect(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	memoryAlignment, offsetAlignment, err := directIOAlignments(file)
	if err != nil {
		return nil, err
	}
	return readDirectFingerprintRangeFromFile(file, count, memoryAlignment, offsetAlignment)
}

func readDirectFingerprintRangeFromFile(file *os.File, count, memoryAlignment, offsetAlignment int) ([]byte, error) {
	if count < 0 {
		return nil, os.ErrInvalid
	}
	if count == 0 {
		return []byte{}, nil
	}
	if memoryAlignment <= 0 {
		memoryAlignment = directIOAlignment
	}
	if offsetAlignment <= 0 {
		offsetAlignment = directIOAlignment
	}

	// The read starts at offset 0 (block-aligned), so only the length is rounded up.
	alignedSize := (count + offsetAlignment - 1) / offsetAlignment * offsetAlignment

	mapping, err := allocDirectIOBuffer(alignedSize + memoryAlignment - 1)
	if err != nil {
		return nil, err
	}
	defer freeDirectIOBuffer(mapping)

	address := uintptr(unsafe.Pointer(&mapping[0]))
	padding := int((uintptr(memoryAlignment) - address%uintptr(memoryAlignment)) % uintptr(memoryAlignment))
	buffer := mapping[padding : padding+alignedSize]

	filled, err := readAligned(file, buffer, memoryAlignment, offsetAlignment)
	if err != nil {
		return nil, err
	}

	if filled > count {
		filled = count
	}
	out := make([]byte, filled)
	copy(out, buffer[:filled])
	return out, nil
}

// readAligned fills buffer with sequential O_DIRECT reads from offset 0. A short
// read leaves the offset unaligned for the next read, so it stops there (or EOF).
func readAligned(file *os.File, buffer []byte, memoryAlignment, offsetAlignment int) (int, error) {
	stride := max(memoryAlignment, offsetAlignment)

	total := 0
	for total < len(buffer) {
		read, err := file.Read(buffer[total:])
		total += read
		if err != nil {
			if errors.Is(err, io.EOF) {
				return total, nil
			}
			return total, err
		}
		if read == 0 || total%stride != 0 {
			return total, nil
		}
	}
	return total, nil
}
