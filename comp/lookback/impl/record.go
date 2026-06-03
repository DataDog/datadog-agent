// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lookbackimpl

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

const recordSize = 24 // context_key(8) + ts_us(8) + value(8)

// record is the in-memory representation of one 24-byte WAL entry.
type record struct {
	contextKey uint64
	tsUs       int64   // Unix microseconds
	value      float64
}

// appendRecord encodes r big-endian into buf and returns the extended slice.
func appendRecord(buf []byte, r record) []byte {
	var tmp [recordSize]byte
	binary.BigEndian.PutUint64(tmp[0:8], r.contextKey)
	binary.BigEndian.PutUint64(tmp[8:16], uint64(r.tsUs))
	binary.BigEndian.PutUint64(tmp[16:24], math.Float64bits(r.value))
	return append(buf, tmp[:]...)
}

// decodeRecord decodes the first recordSize bytes of b into a record.
func decodeRecord(b []byte) (record, error) {
	if len(b) < recordSize {
		return record{}, fmt.Errorf("lookback: short record: %d bytes", len(b))
	}
	return record{
		contextKey: binary.BigEndian.Uint64(b[0:8]),
		tsUs:       int64(binary.BigEndian.Uint64(b[8:16])),
		value:      math.Float64frombits(binary.BigEndian.Uint64(b[16:24])),
	}, nil
}

// readAllRecords reads every complete 24-byte record from r.
// A partial trailing record (e.g. from a crash-truncated file) is silently discarded.
func readAllRecords(r io.Reader) ([]record, error) {
	var out []record
	var buf [recordSize]byte
	for {
		_, err := io.ReadFull(r, buf[:])
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return out, err
		}
		rec, _ := decodeRecord(buf[:])
		out = append(out, rec)
	}
	return out, nil
}
