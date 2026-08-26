// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// This file implements the block-alignment rules that Linux imposes on reads
// through an O_DIRECT descriptor. Everything here is unexported; the only entry
// point is newDirectReader, called by OpenReaderWithFlags in open.go.

package opener

import (
	"errors"
	"fmt"
	"io"
	"os"
	"unsafe"
)

const (
	directIOAlignment = 4096

	// directIOChunkBlocks sizes the chunk that serves small reads, in units of
	// the offset alignment. Four blocks covers the default line fingerprint in a
	// single read while keeping the worst-case over-read for a small byte
	// fingerprint down to a few kilobytes.
	directIOChunkBlocks = 4

	// directIOLargeReadBlocks caps the buffer backing a read too big for the
	// chunk. Without a cap the reader would allocate a second copy of whatever
	// the caller asked for, so a fingerprint configured with a large count would
	// double its peak memory. Capping trades a few more syscalls on such a
	// configuration for a fixed bound.
	directIOLargeReadBlocks = 64
)

// directReader adapts an O_DIRECT descriptor to ordinary Read and Seek
// semantics. The kernel only serves direct reads whose buffer address, file
// offset, and length are all block aligned, while fingerprint configuration
// asks for arbitrary offsets and byte counts.
//
// Reads are lazy and bounded: a Seek costs nothing, and a Read pulls only the
// aligned blocks that overlap the requested range. A configuration that skips
// far into a large file therefore pays for the bytes it hashes rather than for
// everything preceding them.
//
// It is not safe for concurrent use, which is fine because each instance backs
// a single fingerprint computation.
type directReader struct {
	file *os.File

	memoryAlignment int
	offsetAlignment int

	// chunk serves reads smaller than itself, and holds the file bytes in
	// [chunkStart, chunkStart+chunkLength). chunkLength is zero when the chunk
	// holds nothing.
	chunk       []byte
	chunkStart  int64
	chunkLength int

	// scratch backs reads too large for the chunk. It is allocated on first use,
	// so a fingerprint that never takes that path never pays for it.
	scratch []byte

	// position is the logical file offset the next Read starts at. Seek moves it
	// without touching the descriptor.
	position int64
}

// newDirectReader wraps an already-open O_DIRECT descriptor. The reader takes
// ownership of file and closes it on Close.
func newDirectReader(file *os.File) (*directReader, error) {
	memoryAlignment, offsetAlignment := directIOAlignments(file)
	return newDirectReaderWithAlignments(file, memoryAlignment, offsetAlignment)
}

func newDirectReaderWithAlignments(file *os.File, memoryAlignment, offsetAlignment int) (*directReader, error) {
	if file == nil {
		return nil, os.ErrInvalid
	}
	if memoryAlignment <= 0 {
		memoryAlignment = directIOAlignment
	}
	if offsetAlignment <= 0 {
		offsetAlignment = directIOAlignment
	}
	return &directReader{
		file:            file,
		memoryAlignment: memoryAlignment,
		offsetAlignment: offsetAlignment,
		chunk:           newAlignedBuffer(offsetAlignment*directIOChunkBlocks, memoryAlignment),
	}, nil
}

// Read fills p from the current position and may return fewer bytes than asked
// for, since it stops at whichever aligned range it pulled: the rest of the
// chunk for a small request, or the read cap for a large one. Callers needing
// more keep reading; io.ReadFull and bufio.Scanner both do.
func (r *directReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	// A request at least as large as the chunk bypasses it, so a byte
	// fingerprint asking for its whole count does not walk the range a chunk at
	// a time.
	if len(p) >= len(r.chunk) {
		return r.readLarge(p)
	}

	index, ok := r.chunkIndex(r.position)
	if !ok {
		if err := r.refill(); err != nil {
			return 0, err
		}
		if index, ok = r.chunkIndex(r.position); !ok {
			// The chunk covering this offset came back short, so the file ends
			// at or before the current position.
			return 0, io.EOF
		}
	}
	read := copy(p, r.chunk[index:r.chunkLength])
	r.position += int64(read)
	return read, nil
}

// readLarge serves a request too big for the chunk by reading the aligned range
// covering it through the bounded scratch buffer. A request past that bound is
// satisfied partially, which callers already handle: io.ReadFull and
// bufio.Scanner both keep reading until they have what they need.
func (r *directReader) readLarge(p []byte) (int, error) {
	delta := int(r.position % int64(r.offsetAlignment))

	want := len(p)
	if limit := r.offsetAlignment * directIOLargeReadBlocks; want > limit {
		want = limit
	}
	size, ok := roundUpToAlignment(delta+want, r.offsetAlignment)
	if !ok {
		return 0, os.ErrInvalid
	}

	buffer := r.scratchBuffer(size)
	filled, err := r.readAlignedAt(buffer, r.position-int64(delta))
	if err != nil {
		return 0, err
	}
	if filled <= delta {
		return 0, io.EOF
	}
	read := copy(p, buffer[delta:filled])
	r.position += int64(read)
	return read, nil
}

// scratchBuffer returns an aligned buffer of size bytes, reusing the previous
// one when it is already big enough. Successive reads of the same length ask
// for the same size, so a fingerprint allocates here once, and only as much as
// it actually reads through rather than the full cap.
func (r *directReader) scratchBuffer(size int) []byte {
	if len(r.scratch) < size {
		r.scratch = newAlignedBuffer(size, r.memoryAlignment)
	}
	return r.scratch[:size]
}

// refill loads the chunk-sized aligned range containing the current position.
func (r *directReader) refill() error {
	start := r.position - r.position%int64(r.offsetAlignment)
	filled, err := r.readAlignedAt(r.chunk, start)
	if err != nil {
		r.chunkLength = 0
		return err
	}
	r.chunkStart = start
	r.chunkLength = filled
	return nil
}

// chunkIndex reports where a file offset sits inside the current chunk, and
// whether the chunk covers that offset at all.
func (r *directReader) chunkIndex(position int64) (int, bool) {
	if r.chunkLength == 0 || position < r.chunkStart {
		return 0, false
	}
	index := position - r.chunkStart
	if index >= int64(r.chunkLength) {
		return 0, false
	}
	return int(index), true
}

// readAlignedAt fills buffer from start, which must be offset aligned, using
// reads whose offset, length, and buffer address all stay aligned so the kernel
// never rejects the request with EINVAL.
//
// It deliberately avoids os.File.ReadAt: ReadAt fills its buffer by looping over
// pread, so a short read at a non-block-aligned EOF makes it retry at an
// unaligned offset. A single os.File.Read is one read syscall, and for a regular
// file a short read only happens at EOF, so we treat it as such and stop instead
// of issuing that doomed follow-up read.
func (r *directReader) readAlignedAt(buffer []byte, start int64) (int, error) {
	// Advancing only on this boundary keeps both the next offset and the next
	// buffer slice aligned for the larger of the two O_DIRECT requirements.
	stride := r.offsetAlignment
	if r.memoryAlignment > stride {
		stride = r.memoryAlignment
	}

	total := 0
	for total < len(buffer) {
		if _, err := r.file.Seek(start+int64(total), io.SeekStart); err != nil {
			return total, err
		}
		read, err := r.file.Read(buffer[total:])
		total += read
		if err != nil {
			if errors.Is(err, io.EOF) {
				return total, nil
			}
			return total, err
		}
		// A read that returns nothing, or that stops off an alignment boundary,
		// can only be EOF for a regular file; a follow-up read there would be
		// unaligned and O_DIRECT would reject it.
		if read == 0 || total%stride != 0 {
			return total, nil
		}
	}
	return total, nil
}

// Seek moves the logical offset without any I/O, so skipping far into a file is
// free until the following Read pulls the blocks it lands on.
func (r *directReader) Seek(offset int64, whence int) (int64, error) {
	var target int64
	switch whence {
	case io.SeekStart:
		target = offset
	case io.SeekCurrent:
		target = r.position + offset
	case io.SeekEnd:
		info, err := r.file.Stat()
		if err != nil {
			return 0, err
		}
		target = info.Size() + offset
	default:
		return 0, fmt.Errorf("directReader.Seek: invalid whence %d", whence)
	}
	if target < 0 {
		return 0, fmt.Errorf("directReader.Seek: negative position %d", target)
	}
	r.position = target
	return target, nil
}

// Close releases the O_DIRECT descriptor.
func (r *directReader) Close() error {
	return r.file.Close()
}

// newAlignedBuffer returns a size-byte slice whose first element sits on an
// alignment boundary. The Go allocator makes no such guarantee, so the backing
// array is over-allocated and the aligned window is sliced out of it.
func newAlignedBuffer(size, alignment int) []byte {
	raw := make([]byte, size+alignment-1)
	address := uintptr(unsafe.Pointer(&raw[0]))
	padding := int((uintptr(alignment) - address%uintptr(alignment)) % uintptr(alignment))
	return raw[padding : padding+size]
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
