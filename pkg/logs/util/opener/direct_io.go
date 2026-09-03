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

// readDirectFingerprintRange opens path with O_DIRECT and returns the logical
// bytes in [skip, skip+count).
func readDirectFingerprintRange(path string, skip, count int) ([]byte, error) {
	if skip < 0 || count < 0 {
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
	return readDirectFingerprintRangeFromFile(file, skip, count, memoryAlignment, offsetAlignment)
}

func readDirectFingerprintRangeFromFile(file *os.File, skip, count, memoryAlignment, offsetAlignment int) ([]byte, error) {
	if skip < 0 || count < 0 {
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

	alignedStart := skip - skip%offsetAlignment
	logicalEnd := skip + count
	alignedSize := ((logicalEnd - alignedStart) + offsetAlignment - 1) / offsetAlignment * offsetAlignment
	if alignedSize <= 0 {
		return []byte{}, nil
	}

	mapping, err := allocDirectIOBuffer(alignedSize + memoryAlignment - 1)
	if err != nil {
		return nil, err
	}
	defer freeDirectIOBuffer(mapping)

	address := uintptr(unsafe.Pointer(&mapping[0]))
	padding := int((uintptr(memoryAlignment) - address%uintptr(memoryAlignment)) % uintptr(memoryAlignment))
	buffer := mapping[padding : padding+alignedSize]

	filled, err := readAlignedAt(file, buffer, int64(alignedStart), memoryAlignment, offsetAlignment)
	if err != nil {
		return nil, err
	}

	prefix := skip - alignedStart
	if prefix >= filled {
		return []byte{}, nil
	}
	available := filled - prefix
	if available > count {
		available = count
	}
	out := make([]byte, available)
	copy(out, buffer[prefix:prefix+available])
	return out, nil
}

// readAlignedAt fills buffer from start using aligned O_DIRECT reads.
func readAlignedAt(file *os.File, buffer []byte, start int64, memoryAlignment, offsetAlignment int) (int, error) {
	stride := max(memoryAlignment, offsetAlignment)

	total := 0
	for total < len(buffer) {
		if _, err := file.Seek(start+int64(total), io.SeekStart); err != nil {
			return total, err
		}
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
