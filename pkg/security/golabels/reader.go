// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package golabels

import (
	"encoding/binary"
)

// Mirrors collect_go_labels in pkg/security/ebpf/c/include/helpers/span_go.h
// hop for hop, limits included, so that both probes report the same labels:
// change both together.

// Memory reads the memory of a process.
type Memory interface {
	Read(addr uint64, buf []byte) error
}

// ReadStatus mirrors the SPAN_CTX_EVENT_* collect_go_labels returns, see
// pkg/security/ebpf/c/include/constants/custom.h.
type ReadStatus int

// ReadStatus values
const (
	ReadOK ReadStatus = iota
	ReadNone
	ReadNoThreadPointer
	ReadGNotFound
	ReadFault
)

const (
	// goStringSize is the size of a string header: {ptr, len}.
	goStringSize = 16
	// goSliceSize is the size of a slice header: {array, len, cap}.
	goSliceSize = 24
	// goMapBucketSize is GO_MAP_BUCKET_SIZE, the number of slots of a map
	// bucket.
	goMapBucketSize = 8
	// goMapBucketBytes is the size of struct go_map_bucket_t:
	// tophash[8] keys[8] values[8].
	goMapBucketBytes = goMapBucketSize + 2*goMapBucketSize*goStringSize
)

// ReadLabels reads the pprof labels of the goroutine the thread runs, out of
// mem, the memory of its process. threadPointer is the thread pointer of the
// thread, only read when the runtime keeps g in TLS, and gRegister the register
// the Go ABI keeps g in, 0 on an architecture without one.
func (l Layout) ReadLabels(mem Memory, threadPointer uint64, gRegister uint64) ([]Label, ReadStatus) {
	// TLS -> G, with a fallback to the g register where the ABI has one. A
	// tlsOffset of 0 means this binary does not keep g in TLS at all, so the
	// register is the only source.
	var g uint64
	tpMissing := false
	if l.tlsOffset != 0 {
		if threadPointer == 0 {
			tpMissing = true
		} else if word, err := readWord(mem, uint64(int64(threadPointer)+int64(l.tlsOffset))); err == nil {
			g = word
		}
	}
	if g == 0 {
		g = gRegister
	}
	if g == 0 {
		if tpMissing {
			return nil, ReadNoThreadPointer
		}
		return nil, ReadGNotFound
	}

	// G -> M
	m, err := readWord(mem, g+uint64(l.offsets.MOffset))
	if err != nil || m == 0 {
		return nil, ReadFault
	}

	// M -> curg, the goroutine the thread runs
	curg, err := readWord(mem, m+uint64(l.offsets.Curg))
	if err != nil {
		return nil, ReadFault
	}
	if curg == 0 {
		return nil, ReadNone
	}

	// curg -> labels
	labelsPtr, err := readWord(mem, curg+uint64(l.offsets.Labels))
	if err != nil {
		return nil, ReadFault
	}
	if labelsPtr == 0 {
		// the goroutine sets no labels
		return nil, ReadNone
	}

	if l.offsets.HmapBuckets == 0 {
		return readSliceLabels(mem, labelsPtr)
	}
	return l.readMapLabels(mem, labelsPtr)
}

// readSliceLabels reads the labels of Go >= 1.24, a slice of {key, value}
// string headers.
func readSliceLabels(mem Memory, labelsPtr uint64) ([]Label, ReadStatus) {
	var slice [goSliceSize]byte
	if err := mem.Read(labelsPtr, slice[:]); err != nil {
		return nil, ReadFault
	}
	array := binary.NativeEndian.Uint64(slice[0:8])
	numPairs := binary.NativeEndian.Uint64(slice[8:16])
	if numPairs == 0 || array == 0 {
		return nil, ReadNone
	}
	numPairs = min(numPairs, MaxPairs)

	labels := make([]Label, 0, numPairs)
	for i := range numPairs {
		// A pair that fails to read is skipped, as in eBPF.
		var pair [2 * goStringSize]byte
		if err := mem.Read(array+i*2*goStringSize, pair[:]); err != nil {
			continue
		}
		if label, ok := readLabel(mem, pair[:goStringSize], pair[goStringSize:]); ok {
			labels = append(labels, label)
		}
	}
	return labels, ReadOK
}

// readMapLabels reads the labels of Go < 1.24, a map[string]string. Only the
// first bucket is read, as in eBPF: dd-trace-go sets few labels, so they land
// in it.
func (l Layout) readMapLabels(mem Memory, labelsPtr uint64) ([]Label, ReadStatus) {
	hmap, err := readWord(mem, labelsPtr)
	if err != nil || hmap == 0 {
		return nil, ReadFault
	}

	count, err := readWord(mem, hmap+uint64(l.offsets.HmapCount))
	if err != nil || count == 0 {
		return nil, ReadNone
	}

	buckets, err := readWord(mem, hmap+uint64(l.offsets.HmapBuckets))
	if err != nil || buckets == 0 {
		return nil, ReadFault
	}

	var bucket [goMapBucketBytes]byte
	if err := mem.Read(buckets, bucket[:]); err != nil {
		return nil, ReadFault
	}

	labels := make([]Label, 0, goMapBucketSize)
	for slot := range goMapBucketSize {
		// an empty slot
		if bucket[slot] == 0 {
			continue
		}
		keyOff := goMapBucketSize + slot*goStringSize
		valOff := goMapBucketSize + (goMapBucketSize+slot)*goStringSize
		if label, ok := readLabel(mem, bucket[keyOff:keyOff+goStringSize], bucket[valOff:valOff+goStringSize]); ok {
			labels = append(labels, label)
		}
	}
	return labels, ReadOK
}

// readLabel mirrors store_go_label: it reads the key and the value the string
// headers point to, at most KeySize-1 and ValSize-1 bytes of them. A label
// whose key is empty, or whose key or value cannot be read, is dropped.
func readLabel(mem Memory, keyHdr, valHdr []byte) (Label, bool) {
	keyPtr, keyLen := binary.NativeEndian.Uint64(keyHdr[0:8]), binary.NativeEndian.Uint64(keyHdr[8:16])
	if keyPtr == 0 || keyLen == 0 {
		return Label{}, false
	}
	key := make([]byte, min(keyLen, KeySize-1))
	if err := mem.Read(keyPtr, key); err != nil {
		return Label{}, false
	}

	label := Label{Key: string(key)}

	valPtr, valLen := binary.NativeEndian.Uint64(valHdr[0:8]), binary.NativeEndian.Uint64(valHdr[8:16])
	if valPtr != 0 && valLen != 0 {
		val := make([]byte, min(valLen, ValSize-1))
		if err := mem.Read(valPtr, val); err != nil {
			return Label{}, false
		}
		label.Value = string(val)
	}
	return label, true
}

func readWord(mem Memory, addr uint64) (uint64, error) {
	var buf [8]byte
	if err := mem.Read(addr, buf[:]); err != nil {
		return 0, err
	}
	return binary.NativeEndian.Uint64(buf[:]), nil
}
