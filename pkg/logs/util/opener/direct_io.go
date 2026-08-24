// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package opener

import (
	"errors"
	"io"
	"os"
	"sync"
	"unsafe"
)

const directIOAlignment = 4096

// ErrOpenFlagsUnsupported can be returned by platform openers when a filesystem
// rejects the requested read-only open flags.
var ErrOpenFlagsUnsupported = errors.New("requested log open flags are not supported")

// IsOpenFlagsUnsupportedError reports whether a flagged fingerprint open or
// read should be retried once without special flags.
func IsOpenFlagsUnsupportedError(err error) bool {
	return errors.Is(err, ErrOpenFlagsUnsupported) || isOpenFlagsUnsupportedError(err)
}

// directIOFile presents ordinary Reader and Seeker semantics over an O_DIRECT
// descriptor. Linux direct I/O requires aligned memory, offsets, and request
// sizes, while fingerprint configuration intentionally accepts arbitrary byte
// counts and skip offsets.
type directIOFile struct {
	*os.File

	mu              sync.Mutex
	offset          int64
	memoryAlignment int
	offsetAlignment int
	raw             []byte
	aligned         []byte
}

func newDirectIOFile(file *os.File) *directIOFile {
	memoryAlignment, offsetAlignment := directIOAlignments(file)
	return newDirectIOFileWithAlignments(file, memoryAlignment, offsetAlignment)
}

func newDirectIOFileWithAlignments(file *os.File, memoryAlignment, offsetAlignment int) *directIOFile {
	if memoryAlignment <= 0 {
		memoryAlignment = directIOAlignment
	}
	if offsetAlignment <= 0 {
		offsetAlignment = directIOAlignment
	}
	return &directIOFile{
		File:            file,
		memoryAlignment: memoryAlignment,
		offsetAlignment: offsetAlignment,
	}
}

func (f *directIOFile) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	n, err := f.readAtLocked(p, f.offset)
	f.offset += int64(n)
	return n, err
}

func (f *directIOFile) ReadAt(p []byte, offset int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.readAtLocked(p, offset)
}

// writeToChunkSize is the copy chunk WriteTo hands to Read. Any size works
// because Read realigns internally; this is just a reasonable syscall batch.
const writeToChunkSize = 64 * 1024

// WriteTo routes io.Copy through this wrapper's alignment-aware Read.
//
// It must exist: directIOFile embeds *os.File, which has its own WriteTo, and a
// promoted method would satisfy io.WriterTo. io.Copy prefers io.WriterTo over
// Read, so without this override a copy would read the O_DIRECT descriptor
// directly, ignoring both the alignment requirements and the logical offset
// tracked here, which the wrapper deliberately keeps separate from the
// descriptor's own offset.
func (f *directIOFile) WriteTo(w io.Writer) (int64, error) {
	buffer := make([]byte, writeToChunkSize)
	var written int64
	for {
		// Read takes the mutex itself, so it must not be held here.
		read, readErr := f.Read(buffer)
		if read > 0 {
			wrote, writeErr := w.Write(buffer[:read])
			written += int64(wrote)
			if writeErr != nil {
				return written, writeErr
			}
			if wrote != read {
				return written, io.ErrShortWrite
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return written, nil
			}
			return written, readErr
		}
	}
}

func (f *directIOFile) readAtLocked(p []byte, offset int64) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if offset < 0 {
		return 0, os.ErrInvalid
	}
	const maxInt64 = int64(^uint64(0) >> 1)
	if offset > maxInt64-int64(len(p)) {
		return 0, os.ErrInvalid
	}

	alignedOffset := offset - offset%int64(f.offsetAlignment)
	prefix := int(offset - alignedOffset)
	required := prefix + len(p)
	if required < prefix {
		return 0, os.ErrInvalid
	}
	alignedSize, ok := roundUpToAlignment(required, f.offsetAlignment)
	if !ok {
		return 0, os.ErrInvalid
	}
	f.ensureAlignedBuffer(alignedSize)

	n, readErr := f.readAligned(f.aligned[:alignedSize], alignedOffset)
	if n <= prefix {
		if readErr == nil {
			readErr = io.EOF
		}
		return 0, readErr
	}

	copied := copy(p, f.aligned[prefix:n])
	if copied == len(p) {
		return copied, nil
	}
	if readErr == nil {
		readErr = io.EOF
	}
	return copied, readErr
}

// readAligned fills buf starting at offset using positioned reads that always
// keep the file offset, request length, and buffer address aligned, so an
// O_DIRECT descriptor never sees an unaligned request. offset and len(buf) are
// already multiples of the offset alignment.
//
// It deliberately avoids os.File.ReadAt: ReadAt fills its buffer by looping over
// pread, so a short read at a non-block-aligned EOF makes it retry at an
// unaligned offset, which O_DIRECT rejects with EINVAL. A single os.File.Read is
// one read syscall, and for a regular file a short read only happens at EOF, so
// we treat it as such and stop instead of issuing that doomed follow-up read.
func (f *directIOFile) readAligned(buf []byte, offset int64) (int, error) {
	// Advancing only on this boundary keeps both the next offset and the next
	// buffer slice aligned for the largest of the two O_DIRECT requirements.
	stride := f.offsetAlignment
	if f.memoryAlignment > stride {
		stride = f.memoryAlignment
	}

	total := 0
	for total < len(buf) {
		if _, err := f.File.Seek(offset+int64(total), io.SeekStart); err != nil {
			return total, err
		}
		m, err := f.File.Read(buf[total:])
		total += m
		if err != nil {
			if err == io.EOF {
				return total, nil
			}
			return total, err
		}
		// A read that returns nothing, or that stops off an alignment boundary,
		// can only be EOF for a regular file; a follow-up read there would be
		// unaligned and O_DIRECT would reject it.
		if m == 0 || total%stride != 0 {
			return total, nil
		}
	}
	return total, nil
}

func (f *directIOFile) Seek(offset int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var base int64
	switch whence {
	case io.SeekStart:
		base = 0
	case io.SeekCurrent:
		base = f.offset
	case io.SeekEnd:
		info, err := f.File.Stat()
		if err != nil {
			return 0, err
		}
		base = info.Size()
	default:
		return 0, os.ErrInvalid
	}

	next := base + offset
	if next < 0 || (offset > 0 && next < base) || (offset < 0 && next > base) {
		return 0, os.ErrInvalid
	}
	f.offset = next
	return next, nil
}

func (f *directIOFile) ensureAlignedBuffer(size int) {
	if len(f.aligned) >= size {
		return
	}

	f.raw = make([]byte, size+f.memoryAlignment-1)
	address := uintptr(unsafe.Pointer(&f.raw[0]))
	padding := int((uintptr(f.memoryAlignment) - address%uintptr(f.memoryAlignment)) % uintptr(f.memoryAlignment))
	f.aligned = f.raw[padding : padding+size]
}

func roundUpToAlignment(value, alignment int) (int, bool) {
	if value < 0 || alignment <= 0 {
		return 0, false
	}
	maxInt := int(^uint(0) >> 1)
	if value > maxInt-(alignment-1) {
		return 0, false
	}
	return ((value + alignment - 1) / alignment) * alignment, true
}
