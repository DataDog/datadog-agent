// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

package stream

import (
	"bytes"
	"encoding/binary"

	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// ColumnTransaction contains data that is being added to the compressed stream
type ColumnTransaction struct {
	length int
	inputs [][]byte
	buf    [16]byte
}

// Reset prepares transaction to receive a new item
func (ct *ColumnTransaction) Reset() {
	ct.length = 0
	for i := range ct.inputs {
		ct.inputs[i] = ct.inputs[i][0:0]
	}
}

// Write adds a sequence of bytes to a column
func (ct *ColumnTransaction) Write(column int, b []byte) {
	ct.inputs[column] = append(ct.inputs[column], b...)
	ct.length += len(b)
}

// Uint64 adds a varint encoded unsigned integer to a column
func (ct *ColumnTransaction) Uint64(column int, v uint64) {
	n := binary.PutUvarint(ct.buf[:], v)
	ct.Write(column, ct.buf[:n])
}

// Int64 adds a varint encoded integer to a column
func (ct *ColumnTransaction) Int64(column int, v int64) {
	ct.Uint64(column, uint64(v))
}

// Sint64 adds a zigzag encoded integer to a column
func (ct *ColumnTransaction) Sint64(column int, v int64) {
	u := uint64(v<<1) ^ uint64(v>>63)
	ct.Uint64(column, u)
}

// Float32 adds a little-endian encoded float32 value to a column
func (ct *ColumnTransaction) Float32(column int, v float32) {
	n, _ := binary.Encode(ct.buf[:], binary.LittleEndian, v)
	ct.Write(column, ct.buf[:n])
}

// Float64 adds a little-endian encoded float64 value to a column
func (ct *ColumnTransaction) Float64(column int, v float64) {
	n, _ := binary.Encode(ct.buf[:], binary.LittleEndian, v)
	ct.Write(column, ct.buf[:n])
}

type column struct {
	length     int
	input      []byte
	output     bytes.Buffer
	compressor compression.StreamCompressor
}

func (c *column) add(data []byte) int {
	c.input = append(c.input, data...)
	c.length += len(data)
	return len(data)
}

func (c *column) pack(compression metricscompression.Component) (int, error) {
	if len(c.input) == 0 {
		return 0, nil
	}

	if c.compressor == nil {
		c.output.Reset()
		c.compressor = compression.NewStreamCompressor(&c.output)
	}

	prevLen := c.output.Len()
	_, err := c.compressor.Write(c.input)
	if err != nil {
		return 0, err
	}
	c.compressor.Flush()
	c.input = c.input[0:0]

	return c.output.Len() - prevLen, nil
}

func (c *column) reset() {
	c.length = 0
}

func (c *column) finish() error {
	if c.compressor != nil {
		err := c.compressor.Close()
		c.compressor = nil
		if err != nil {
			return err
		}
	}

	return nil
}

// ColumnCompressor builds columnar payloads while observing compressed and uncompressed size limits.
type ColumnCompressor struct {
	columns []column

	totalLength  int
	inputLength  int
	outputLength int

	maxCompressedSize   int
	maxUncompressedSize int

	compression metricscompression.Component
}

// NewColumnCompressor creates a new instance
func NewColumnCompressor(compression metricscompression.Component, numColumns int, maxCompressedSize, maxUncompressedSize int) ColumnCompressor {
	columns := make([]column, numColumns)

	return ColumnCompressor{
		columns: columns,

		maxCompressedSize:   maxCompressedSize,
		maxUncompressedSize: maxUncompressedSize,

		compression: compression,
	}
}

func (cc *ColumnCompressor) hasRoomForItem(txn *ColumnTransaction) bool {
	compressBound := cc.compression.CompressBound(cc.inputLength + txn.length)
	return compressBound+cc.outputLength < cc.maxCompressedSize && txn.length+cc.totalLength < cc.maxUncompressedSize
}

// AddItem tries to add a transaction to the compressed payload.
//
// Returns ErrPayloadFull if the transaction does not fit within current limits.
func (cc *ColumnCompressor) AddItem(txn *ColumnTransaction) error {
	if !cc.hasRoomForItem(txn) {
		if cc.inputLength == 0 {
			return ErrItemTooBig
		}

		err := cc.pack()
		if err != nil {
			return err
		}

		if !cc.hasRoomForItem(txn) {
			return ErrPayloadFull
		}
	}

	for i, buffer := range txn.inputs {
		n := cc.columns[i].add(buffer)
		cc.inputLength += n
		cc.totalLength += n
	}

	return nil
}

func (cc *ColumnCompressor) pack() error {
	for i := range cc.columns {
		n, err := cc.columns[i].pack(cc.compression)
		if err != nil {
			return err
		}
		cc.inputLength = 0
		cc.outputLength += n
	}

	return nil
}

// Reset clears compressor state and prepares it to build a new payload.
func (cc *ColumnCompressor) Reset() {
	cc.totalLength = 0

	for i := range cc.columns {
		cc.columns[i].reset()
	}
}

// Close finishes compression of all pending data.
func (cc *ColumnCompressor) Close() error {
	err := cc.pack()
	if err != nil {
		return err
	}

	for i := range cc.columns {
		err := cc.columns[i].finish()
		if err != nil {
			return err
		}
	}

	cc.outputLength = 0

	return nil
}

// UncompressedLen returns length of uncompressed data in a column.
func (cc *ColumnCompressor) UncompressedLen(column int) int {
	return cc.columns[column].length
}

// CompressedBytes returns compressed bytes for a column.
func (cc *ColumnCompressor) CompressedBytes(column int) []byte {
	return cc.columns[column].output.Bytes()
}

// NewTransaction creates a new transaction for the compressor.
func (cc *ColumnCompressor) NewTransaction() *ColumnTransaction {
	return &ColumnTransaction{
		inputs: make([][]byte, len(cc.columns)),
	}
}
