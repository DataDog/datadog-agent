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

const (
	directIOAlignment = 4096

	// directIODefaultWindowBlocks sizes progressive line-mode reads in units of
	// the offset alignment. Four blocks covers typical line fingerprints in one
	// syscall while keeping small byte-range over-read down to a few kilobytes.
	directIODefaultWindowBlocks = 4

	// directIOMaxWindowBlocks caps each progressive read. A large max_bytes limit
	// therefore costs a few syscalls, not one allocation sized to the limit.
	directIOMaxWindowBlocks = 64
)

// directFingerprintReader is a forward-only io.ReadCloser over [0, limit). It
// pulls aligned kernel blocks progressively and never seeks.
type directFingerprintReader struct {
	file *os.File

	memoryAlignment int
	offsetAlignment int

	window        []byte
	windowStart   int64
	windowLength  int
	windowMapping []byte

	position int64
	limit    int64
}

// openDirectFingerprintStream wraps an O_DIRECT descriptor for line-mode
// fingerprinting over [0, limit).
func openDirectFingerprintStream(file *os.File, limit int) (*directFingerprintReader, error) {
	if file == nil {
		return nil, os.ErrInvalid
	}
	if limit < 0 {
		return nil, os.ErrInvalid
	}
	memoryAlignment, offsetAlignment, err := directIOAlignments(file)
	if err != nil {
		return nil, err
	}
	return openDirectFingerprintStreamWithAlignments(file, limit, memoryAlignment, offsetAlignment)
}

func openDirectFingerprintStreamWithAlignments(file *os.File, limit, memoryAlignment, offsetAlignment int) (*directFingerprintReader, error) {
	if memoryAlignment <= 0 {
		memoryAlignment = directIOAlignment
	}
	if offsetAlignment <= 0 {
		offsetAlignment = directIOAlignment
	}
	reader := &directFingerprintReader{
		file:            file,
		memoryAlignment: memoryAlignment,
		offsetAlignment: offsetAlignment,
		limit:           int64(limit),
	}
	if err := reader.resizeWindow(offsetAlignment * directIODefaultWindowBlocks); err != nil {
		return nil, err
	}
	return reader, nil
}

// Read fills p from the current position up to limit.
func (r *directFingerprintReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.position >= r.limit {
		return 0, io.EOF
	}
	if int64(len(p)) > r.limit-r.position {
		p = p[:r.limit-r.position]
	}

	index, ok := r.windowIndex(r.position)
	if !ok {
		if err := r.refill(len(p)); err != nil {
			return 0, err
		}
		if index, ok = r.windowIndex(r.position); !ok {
			return 0, io.EOF
		}
	}
	read := copy(p, r.window[index:r.windowLength])
	r.position += int64(read)
	if read == 0 {
		return 0, io.EOF
	}
	if r.position >= r.limit {
		return read, io.EOF
	}
	return read, nil
}

func (r *directFingerprintReader) refill(requested int) error {
	delta := int(r.position % int64(r.offsetAlignment))

	want := min(requested, r.offsetAlignment*directIOMaxWindowBlocks-delta)
	remaining := int(r.limit - r.position)
	if remaining < want {
		want = remaining
	}
	size := (delta + want + r.offsetAlignment - 1) / r.offsetAlignment * r.offsetAlignment
	size = max(size, r.offsetAlignment*directIODefaultWindowBlocks)
	if len(r.window) < size {
		if err := r.resizeWindow(size); err != nil {
			return err
		}
	}

	start := r.position - int64(delta)
	filled, err := readAlignedAt(r.file, r.window[:size], start, r.memoryAlignment, r.offsetAlignment)
	if err != nil {
		r.windowLength = 0
		return err
	}
	r.windowStart = start
	r.windowLength = filled
	return nil
}

func (r *directFingerprintReader) windowIndex(position int64) (int, bool) {
	if r.windowLength == 0 || position < r.windowStart {
		return 0, false
	}
	index := position - r.windowStart
	if index >= int64(r.windowLength) {
		return 0, false
	}
	return int(index), true
}

func (r *directFingerprintReader) Close() error {
	freeDirectIOBuffer(r.windowMapping)
	r.windowMapping = nil
	r.window = nil
	r.windowLength = 0
	return r.file.Close()
}

func (r *directFingerprintReader) resizeWindow(size int) error {
	mapping, err := allocDirectIOBuffer(size + r.memoryAlignment - 1)
	if err != nil {
		return err
	}

	address := uintptr(unsafe.Pointer(&mapping[0]))
	padding := int((uintptr(r.memoryAlignment) - address%uintptr(r.memoryAlignment)) % uintptr(r.memoryAlignment))

	freeDirectIOBuffer(r.windowMapping)
	r.windowMapping = mapping
	r.window = mapping[padding : padding+size]
	return nil
}

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
