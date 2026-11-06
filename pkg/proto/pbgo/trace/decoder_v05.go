// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"errors"
	"fmt"

	"github.com/tinylib/msgp/msgp"
)

// dictionaryString reads an int from decoder dc and returns the string
// at that index from dict.
func dictionaryString(bts []byte, dict []string) (string, []byte, error) {
	var (
		ui  uint32
		err error
	)
	ui, bts, err = msgp.ReadUint32Bytes(bts)
	if err != nil {
		return "", bts, err
	}
	idx := int(ui)
	if idx >= len(dict) {
		return "", bts, fmt.Errorf("dictionary index %d out of range", idx)
	}
	return dict[idx], bts, nil
}

// UnmarshalMsgDictionary decodes a trace using the specification from the v0.5 endpoint.
// For details, see the documentation for endpoint v0.5 in pkg/trace/api/version.go
func (t *Traces) UnmarshalMsgDictionary(bts []byte) error {
	var err error
	if _, bts, err = safeReadHeaderBytes(bts, msgp.ReadArrayHeaderBytes); err != nil {
		return err
	}
	// read dictionary
	var sz uint32
	if sz, bts, err = safeReadDictionaryHeaderBytes(bts); err != nil {
		return err
	}
	dict := make([]string, sz)
	for i := range dict {
		var str string
		str, bts, err = parseStringBytes(bts)
		if err != nil {
			return err
		}
		dict[i] = str
	}
	// read traces
	sz, bts, err = safeReadHeaderBytes(bts, msgp.ReadArrayHeaderBytes)
	if err != nil {
		return err
	}
	if cap(*t) >= int(sz) {
		*t = (*t)[:sz]
	} else {
		*t = make(Traces, sz)
	}
	for i := range *t {
		sz, bts, err = safeReadHeaderBytes(bts, msgp.ReadArrayHeaderBytes)
		if err != nil {
			return err
		}
		if cap((*t)[i]) >= int(sz) {
			(*t)[i] = (*t)[i][:sz]
		} else {
			(*t)[i] = make(Trace, sz)
		}
		for j := range (*t)[i] {
			if (*t)[i][j] == nil {
				(*t)[i][j] = new(Span)
			}
			if bts, err = (*t)[i][j].UnmarshalMsgDictionary(bts, dict); err != nil {
				return err
			}
		}
	}
	return nil
}

// spanPropertyCount specifies the number of top-level properties that a span
// has.
const spanPropertyCount = 12

// UnmarshalMsgDictionary decodes a span from the given decoder dc, looking up strings
// in the given dictionary dict. For details, see the documentation for endpoint v0.5
// in pkg/trace/api/version.go
func (z *Span) UnmarshalMsgDictionary(bts []byte, dict []string) ([]byte, error) {
	var (
		sz  uint32
		err error
	)
	sz, bts, err = safeReadHeaderBytes(bts, msgp.ReadArrayHeaderBytes)
	if err != nil {
		return bts, err
	}
	if sz != spanPropertyCount {
		return bts, errors.New("encoded span needs exactly 12 elements in array")
	}
	// Service (0)
	z.Service, bts, err = dictionaryString(bts, dict)
	if err != nil {
		return bts, err
	}
	// Name (1)
	z.Name, bts, err = dictionaryString(bts, dict)
	if err != nil {
		return bts, err
	}
	// Resource (2)
	z.Resource, bts, err = dictionaryString(bts, dict)
	if err != nil {
		return bts, err
	}
	// TraceID (3)
	z.TraceID, bts, err = parseUint64Bytes(bts)
	if err != nil {
		return bts, err
	}
	// SpanID (4)
	z.SpanID, bts, err = parseUint64Bytes(bts)
	if err != nil {
		return bts, err
	}
	// ParentID (5)
	z.ParentID, bts, err = parseUint64Bytes(bts)
	if err != nil {
		return bts, err
	}
	// Start (6)
	z.Start, bts, err = parseInt64Bytes(bts)
	if err != nil {
		return bts, err
	}
	// Duration (7)
	z.Duration, bts, err = parseInt64Bytes(bts)
	if err != nil {
		return bts, err
	}
	// Error (8)
	z.Error, bts, err = parseInt32Bytes(bts)
	if err != nil {
		return bts, err
	}
	// Meta (9)
	sz, bts, err = safeReadHeaderBytes(bts, msgp.ReadMapHeaderBytes)
	if err != nil {
		return bts, err
	}
	if err = checkMapEntryCount(sz, bts); err != nil {
		return bts, err
	}
	if z.Meta == nil && sz > 0 {
		z.Meta = make(map[string]string, sz)
	} else if len(z.Meta) > 0 {
		for key := range z.Meta {
			delete(z.Meta, key)
		}
	}
	for sz > 0 {
		sz--
		var key, val string
		key, bts, err = dictionaryString(bts, dict)
		if err != nil {
			return bts, err
		}
		val, bts, err = dictionaryString(bts, dict)
		if err != nil {
			return bts, err
		}
		z.Meta[key] = val
	}
	// Metrics (10)
	sz, bts, err = safeReadHeaderBytes(bts, msgp.ReadMapHeaderBytes)
	if err != nil {
		return bts, err
	}
	if err = checkMapEntryCount(sz, bts); err != nil {
		return bts, err
	}
	if z.Metrics == nil && sz > 0 {
		z.Metrics = make(map[string]float64, sz)
	} else if len(z.Metrics) > 0 {
		for key := range z.Metrics {
			delete(z.Metrics, key)
		}
	}
	for sz > 0 {
		sz--
		var (
			key string
			val float64
		)
		key, bts, err = dictionaryString(bts, dict)
		if err != nil {
			return bts, err
		}
		val, bts, err = parseFloat64Bytes(bts)
		if err != nil {
			return bts, err
		}
		z.Metrics[key] = val
	}
	// Type (11)
	z.Type, bts, err = dictionaryString(bts, dict)
	if err != nil {
		return bts, err
	}
	return bts, nil
}

const (
	// maxV05CollectionSize bounds the element count of every array or map header in a v0.5 payload other
	// than the dictionary. It matches the `//msgp:limit arrays:500000 maps:500000` directive in trace.go
	// that applies to the generated v0.4 decoders.
	maxV05CollectionSize = 500_000
	// maxV05DictionarySize bounds the number of strings in the v0.5 dictionary.
	maxV05DictionarySize = 25 * 1e6
)

var errTooLongPayload = errors.New("too long payload")

// checkMapEntryCount rejects a map header whose entry count couldn't be backed by the remaining bytes:
// every entry reads at least two msgpack values (a key and a value), each at least one byte.
func checkMapEntryCount(n uint32, remaining []byte) error {
	if uint64(n)*2 > uint64(len(remaining)) {
		return msgp.ErrShortBytes
	}
	return nil
}

// safeReadHeaderBytes wraps msgp header readers (typically ReadArrayHeaderBytes and ReadMapHeaderBytes).
// It protects the caller from making unbounded allocations through `make(any, sz)` by rejecting counts above
// maxV05CollectionSize or larger than the bytes remaining in the buffer.
func safeReadHeaderBytes(b []byte, read func([]byte) (uint32, []byte, error)) (uint32, []byte, error) {
	return readBoundedHeaderBytes(b, read, maxV05CollectionSize)
}

// safeReadDictionaryHeaderBytes is safeReadHeaderBytes for the dictionary, which keeps the larger
// maxV05DictionarySize limit.
func safeReadDictionaryHeaderBytes(b []byte) (uint32, []byte, error) {
	return readBoundedHeaderBytes(b, msgp.ReadArrayHeaderBytes, maxV05DictionarySize)
}

// readBoundedHeaderBytes reads an array or map header with `read` and rejects counts that are either above
// `limit` or larger than the number of bytes left in the buffer. Every msgpack value takes at least one byte,
// so a count larger than the remaining input can never be satisfied; rejecting it here keeps callers from
// sizing a make(...) from a count the payload cannot back.
func readBoundedHeaderBytes(b []byte, read func([]byte) (uint32, []byte, error), limit uint32) (uint32, []byte, error) {
	sz, bts, err := read(b)
	if err != nil {
		return 0, nil, err
	}
	if sz > limit {
		return 0, nil, errTooLongPayload
	}
	if uint64(sz) > uint64(len(bts)) {
		return 0, nil, msgp.ErrShortBytes
	}
	return sz, bts, nil
}
