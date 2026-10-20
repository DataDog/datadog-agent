// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package ntfsmft

import (
	"container/heap"
	"sort"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// -------------------------------------------------------------------------
// Top-N file heap (min-heap on Size)
// -------------------------------------------------------------------------

// fileCandidate is a top-N file candidate captured during pass 2. The basename
// is copied from the per-record buffer at heap-push time so it survives past
// the streamPipelined callback that produced it.
type fileCandidate struct {
	idx      uint64
	sequence uint16
	size     int64
	basename string // decoded once at heap insertion
}

// fileHeap is a min-heap of fileCandidate by size. We keep the smallest at the
// top so a bigger candidate can pop it.
type fileHeap []fileCandidate

func (h fileHeap) Len() int            { return len(h) }
func (h fileHeap) Less(i, j int) bool  { return h[i].size < h[j].size }
func (h fileHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *fileHeap) Push(x interface{}) { *h = append(*h, x.(fileCandidate)) }
func (h *fileHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// topFiles maintains the top-N files seen so far by size. After the scan,
// drained() returns the candidates in descending size order.
type topFiles struct {
	cap     int
	minSize int64    // candidates < minSize are ignored; raised by heap as it fills
	heap    fileHeap // capacity = cap
}

func newTopFiles(n int, minSize int64) *topFiles {
	if n <= 0 {
		return nil
	}
	return &topFiles{cap: n, minSize: minSize, heap: make(fileHeap, 0, n)}
}

// consider runs in the pass-3 hot path. It must be cheap when the candidate
// won't qualify; that case is a single int comparison plus an early return.
// When a candidate qualifies we decode its basename from the (still-valid)
// nameBytes slice — that's the only allocation, and it's bounded by the
// total number of distinct heap insertions over the scan (~O(N · log(total))
// in the random-input case; in practice closer to O(N) once threshold rises).
func (t *topFiles) consider(idx uint64, e *mftEntry, size int64) {
	if t == nil || size < t.minSize {
		return
	}
	if len(t.heap) < t.cap {
		heap.Push(&t.heap, fileCandidate{
			idx:      idx,
			sequence: e.sequence,
			size:     size,
			basename: decodeUTF16Name(e.nameBytes),
		})
		if len(t.heap) == t.cap && t.heap[0].size > t.minSize {
			t.minSize = t.heap[0].size
		}
		return
	}
	if size <= t.heap[0].size {
		return
	}
	t.heap[0] = fileCandidate{
		idx:      idx,
		sequence: e.sequence,
		size:     size,
		basename: decodeUTF16Name(e.nameBytes),
	}
	heap.Fix(&t.heap, 0)
	t.minSize = t.heap[0].size
}

// drained returns the candidates in descending size order (largest first).
func (t *topFiles) drained() []fileCandidate {
	if t == nil {
		return nil
	}
	out := make([]fileCandidate, len(t.heap))
	copy(out, t.heap)
	sort.Slice(out, func(i, j int) bool {
		if out[i].size != out[j].size {
			return out[i].size > out[j].size
		}
		return out[i].basename < out[j].basename
	})
	return out
}

// -------------------------------------------------------------------------
// Extension aggregator
// -------------------------------------------------------------------------

// extAggregator counts total bytes and file count per file extension.
type extAggregator struct {
	bySize  map[string]int64
	byCount map[string]int
}

func newExtAggregator(enabled bool) *extAggregator {
	if !enabled {
		return nil
	}
	// Most volumes have hundreds of distinct extensions; pre-size to avoid
	// rehashing during the scan.
	return &extAggregator{
		bySize:  make(map[string]int64, 512),
		byCount: make(map[string]int, 512),
	}
}

// addFromName extracts an extension from the raw UTF-16 name and aggregates
// size. ASCII extensions use the allocation-free path; a non-ASCII extension
// falls back to a complete UTF-16 decode so it remains a distinct bucket.
//
// Hot path: must not allocate. Uses a stack array + the compiler's
// `m[string(byteSlice)]` optimization to update the map in place.
func (a *extAggregator) addFromName(nameBytes []byte, size int64) {
	if a == nil {
		return
	}
	// A $FILE_NAME name length is a uint8 count of UTF-16 code units. An
	// extension can therefore contain at most 254 ASCII bytes (one byte is the
	// separating dot). Keep the entire legal ASCII extension on the stack
	// instead of imposing an arbitrary classification limit.
	var buf [255]byte
	n := extractAsciiExtension(nameBytes, buf[:])
	if n == nonASCIIExtension {
		ext := decodedExtension(nameBytes)
		if ext != "" {
			a.bySize[ext] += size
			a.byCount[ext]++
			return
		}
	}
	if n < 0 {
		// No extension (or malformed UTF-16 that decoded without one) — track
		// it under "" so totals are honest.
		a.bySize[""] += size
		a.byCount[""]++
		return
	}
	// `m[string(buf[:n])]` is recognized by the Go compiler and does NOT
	// allocate when the key already exists in the map. New keys allocate
	// once each (bounded by # distinct extensions).
	a.bySize[string(buf[:n])] += size
	a.byCount[string(buf[:n])]++
}

// ExtensionEntry is one extension and its aggregated stats.
type ExtensionEntry struct {
	Ext   string // lowercased, no leading dot; "" = no extension
	Size  int64
	Count int
}

func (a *extAggregator) topN(n int, minSize int64) []ExtensionEntry {
	if a == nil {
		return nil
	}
	out := make([]ExtensionEntry, 0, len(a.bySize))
	for ext, size := range a.bySize {
		if size < minSize {
			continue
		}
		out = append(out, ExtensionEntry{Ext: ext, Size: size, Count: a.byCount[ext]})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Size != out[j].Size {
			return out[i].Size > out[j].Size
		}
		return out[i].Ext < out[j].Ext
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

// -------------------------------------------------------------------------
// Name helpers
// -------------------------------------------------------------------------

// decodeUTF16Name converts a UTF-16 little-endian byte slice (NOT null-
// terminated) to a Go string. Used only for top-N heap entries — bounded.
func decodeUTF16Name(b []byte) string {
	if len(b) < 2 {
		return ""
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[i*2]) | uint16(b[i*2+1])<<8
	}
	return windows.UTF16ToString(u)
}

const (
	noExtension       = -1
	nonASCIIExtension = -2
	extensionTooLong  = -3
)

// extractAsciiExtension finds the lowercased ASCII extension in a raw UTF-16
// name (no leading dot returned). A leading run of dots is part of a dotfile
// name, not an extension separator: .claude and ..claude have no extension,
// while .claude.json has extension json. It scans the complete suffix, so every
// legal $FILE_NAME extension is considered. It writes the result to out and
// returns its byte count; it returns noExtension for no/empty extension,
// nonASCIIExtension when the suffix needs a complete UTF-16 decode, or
// extensionTooLong when out cannot hold the ASCII result.
func extractAsciiExtension(nameUTF16, out []byte) int {
	if len(nameUTF16) < 4 {
		return noExtension
	}
	end := len(nameUTF16) &^ 1
	for i := end - 2; i >= 0; i -= 2 {
		hi := nameUTF16[i+1]
		lo := nameUTF16[i]
		// ASCII is U+0000 through U+007F. A zero high byte alone is not
		// sufficient: U+0080 through U+00FF also fit in the low byte.
		if hi != 0 || lo >= 0x80 {
			return nonASCIIExtension
		}
		if lo == '.' {
			// A dot starts an extension only after a non-dot basename character.
			// This recognizes dotfiles without decoding the UTF-16 name.
			hasBasename := false
			for j := 0; j < i; j += 2 {
				if nameUTF16[j] != '.' || nameUTF16[j+1] != 0 {
					hasBasename = true
					break
				}
			}
			if !hasBasename {
				return noExtension
			}
			tailLen := (end - (i + 2)) / 2
			if tailLen == 0 {
				return noExtension
			}
			if tailLen > len(out) {
				return extensionTooLong
			}
			for j := 0; j < tailLen; j++ {
				ch := nameUTF16[i+2+j*2]
				if ch >= 'A' && ch <= 'Z' {
					ch += 32
				}
				out[j] = ch
			}
			return tailLen
		}
	}
	return noExtension
}

// decodedExtension is the cold path for a suffix containing non-ASCII UTF-16.
// It deliberately uses the same literal '.' delimiter as the raw ASCII path.
func decodedExtension(nameUTF16 []byte) string {
	name := decodeUTF16Name(nameUTF16)
	dot := strings.LastIndexByte(name, '.')
	if dot < 0 || dot == len(name)-1 || strings.Trim(name[:dot], ".") == "" {
		return ""
	}
	return strings.ToLower(name[dot+1:])
}

// -------------------------------------------------------------------------
// Path resolution via OpenFileByID
// -------------------------------------------------------------------------

// fileIDDescriptor mirrors Windows FILE_ID_DESCRIPTOR. The struct size is
// 24 bytes on x64: dwSize(4) + Type(4) + 16-byte union (FILE_ID_128 is the
// largest variant). For Type=FileIdType (0), we use the first 8 bytes of
// the union as the 64-bit NTFS file reference: low 48 bits = MFT idx, high
// 16 bits = sequence number.
type fileIDDescriptor struct {
	Size   uint32
	Type   uint32 // 0 = FileIdType
	FileID uint64
	_pad   uint64 // pad to FILE_ID_128 union size
}

// resolveCandidatePaths opens each candidate by FILE_ID and translates it to
// a Win32 path via GetFinalPathNameByHandle. On failure (file deleted between
// scan and resolution, permission, etc.) the basename is used as a fallback.
//
// Bounded by the size of the heap (≤ topN). ~2 syscalls per file; for default
// N=25 this is well under a millisecond.
func resolveCandidatePaths(volumeRoot string, candidates []fileCandidate) []FileEntry {
	if len(candidates) == 0 {
		return nil
	}
	rootW, err := windows.UTF16PtrFromString(volumeRoot)
	if err != nil {
		return fallbackPaths(candidates)
	}
	// This handle is never read; OpenFileById uses it only to identify the volume,
	// so it asks for no access at all. That also makes the open itself more likely
	// to succeed, and a failure here costs every top-file path (see fallbackPaths).
	// FILE_FLAG_BACKUP_SEMANTICS is required to get a directory handle.
	// TestOpenFileByIdVolumeHintAccessMask checks this mask still resolves IDs.
	hRoot, err := windows.CreateFile(
		rootW,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return fallbackPaths(candidates)
	}
	defer windows.CloseHandle(hRoot)

	// OpenFileById has no wrapper in golang.org/x/sys/windows, so it is the one
	// call here that must be resolved dynamically.
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	openByID := kernel32.NewProc("OpenFileById")

	out := make([]FileEntry, 0, len(candidates))
	// 32K wchars covers any Windows extended-length path (\\?\ + 32767).
	const maxPathChars = 32768
	pathBuf := make([]uint16, maxPathChars)
	for _, c := range candidates {
		fid := fileIDDescriptor{
			Size:   uint32(unsafe.Sizeof(fileIDDescriptor{})),
			Type:   0,
			FileID: (uint64(c.sequence) << 48) | (c.idx & 0x0000FFFFFFFFFFFF),
		}
		hr, _, _ := openByID.Call(
			uintptr(hRoot),
			uintptr(unsafe.Pointer(&fid)),
			uintptr(windows.FILE_READ_ATTRIBUTES),
			uintptr(windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE),
			0,
			uintptr(windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT),
		)
		h := windows.Handle(hr)
		// INVALID_HANDLE_VALUE on x64 is ^uintptr(0). Treat any handle that
		// is 0 or invalid as failure; otherwise the call succeeded regardless
		// of GetLastError state.
		if hr == 0 || h == windows.InvalidHandle {
			out = append(out, FileEntry{Path: "?\\" + c.basename, Size: c.size})
			continue
		}
		// Read creation / last-write times from the handle we already hold
		// (opened with FILE_READ_ATTRIBUTES, which is all this needs). This is
		// one extra syscall per displayed file — bounded by topN + find limits,
		// not the per-record hot path. Best-effort: on failure the times stay
		// zero and the entry is still emitted with its path.
		var created, modified time.Time
		var info windows.ByHandleFileInformation
		if windows.GetFileInformationByHandle(h, &info) == nil {
			created = time.Unix(0, info.CreationTime.Nanoseconds()).UTC()
			modified = time.Unix(0, info.LastWriteTime.Nanoseconds()).UTC()
		}
		// 0 flags = VOLUME_NAME_DOS | FILE_NAME_NORMALIZED.
		//
		// The return value is ambiguous by design and the wrapper only turns the
		// first case into an error: 0 means failure, a value below the buffer size
		// is the count written, and a value at or above it is the REQUIRED size
		// instead. So the bound has to be checked separately from err.
		n, err := windows.GetFinalPathNameByHandle(h, &pathBuf[0], maxPathChars, 0)
		windows.CloseHandle(h)
		if err != nil || n >= maxPathChars {
			out = append(out, FileEntry{Path: "?\\" + c.basename, Size: c.size, Created: created, Modified: modified})
			continue
		}
		path := windows.UTF16ToString(pathBuf[:n])
		// VOLUME_NAME_DOS returns a \\?\-prefixed path; users expect "C:\..."
		// not "\\?\C:\...".
		path = stripExtendedPathPrefix(path)
		out = append(out, FileEntry{Path: path, Size: c.size, Created: created, Modified: modified})
	}
	return out
}

func fallbackPaths(candidates []fileCandidate) []FileEntry {
	out := make([]FileEntry, len(candidates))
	for i, c := range candidates {
		out[i] = FileEntry{Path: "?\\" + c.basename, Size: c.size}
	}
	return out
}

func stripExtendedPathPrefix(p string) string {
	const prefix = `\\?\`
	if len(p) >= len(prefix) && p[:len(prefix)] == prefix {
		return p[len(prefix):]
	}
	return p
}

// FileEntry is one large-file entry in Result.TopFiles or a --find match.
type FileEntry struct {
	Path string
	Size int64
	// Created and Modified are the file's creation and last-data-modification
	// times in UTC, read from the file's $STANDARD_INFORMATION via
	// GetFileInformationByHandle during post-scan path resolution. Zero when
	// the file could not be opened (deleted between scan and resolution, etc.).
	Created  time.Time
	Modified time.Time
}
