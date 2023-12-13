// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

//go:build zlib

package stream

import (
	"bytes"
	"compress/zlib"
	"errors"
	"expvar"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

const (
	// Available is true if the code is compiled in
	Available = true
)

var (
	compressorExpvars    = expvar.NewMap("compressor")
	expvarsTotalPayloads = expvar.Int{}
	expvarsTotalCycles   = expvar.Int{}
	expvarsBytesIn       = expvar.Int{}
	expvarsBytesOut      = expvar.Int{}

	tlmTotalPayloads = telemetry.NewCounter("compressor", "total_payloads",
		nil, "Total payloads in the compressor serializer")
	tlmTotalCycles = telemetry.NewCounter("compressor", "total_cycles",
		nil, "Total cycles in the compressor serializer")
	tlmBytesIn = telemetry.NewCounter("compressor", "bytes_in",
		nil, "Count of bytes entering the compressor serializer")
	tlmBytesOut = telemetry.NewCounter("compressor", "bytes_out",
		nil, "Count of bytes out the compressor serializer")
)

var (
	maxRepacks = 40 // CPU time vs tighter payload tradeoff
)

var (
	// ErrPayloadFull is returned when the payload buffer is full
	ErrPayloadFull = errors.New("reached maximum payload size")

	// ErrItemTooBig is returned when a item alone exceeds maximum payload size
	ErrItemTooBig = errors.New("item alone exceeds maximum payload size")
)

func init() {
	compressorExpvars.Set("TotalPayloads", &expvarsTotalPayloads)
	compressorExpvars.Set("TotalCompressCycles", &expvarsTotalCycles)
	compressorExpvars.Set("BytesIn", &expvarsBytesIn)
	compressorExpvars.Set("BytesOut", &expvarsBytesOut)
}

// Compressor is in charge of compressing items for a single payload
type Compressor struct {
	input               *bytes.Buffer // temporary buffer for data that has not been compressed yet
	compressed          *bytes.Buffer // output buffer containing the compressed payload
	zipper              *zlib.Writer
	header              []byte // json header to print at the beginning of the payload
	footer              []byte // json footer to append at the end of the payload
	uncompressedWritten int    // uncompressed bytes written
	firstItem           bool   // tells if the first item has been written
	repacks             int    // numbers of time we had to pack this payload
	maxUnzippedItemSize int
	maxZippedItemSize   int
	maxPayloadSize      int
	maxUncompressedSize int
	separator           []byte
}

// NewCompressor returns a new instance of a Compressor
func NewCompressor(input, output *bytes.Buffer, maxPayloadSize, maxUncompressedSize int, header, footer []byte, separator []byte) (*Compressor, error) {
	c := &Compressor{
		header:              header,
		footer:              footer,
		input:               input,
		compressed:          output,
		firstItem:           true,
		maxPayloadSize:      maxPayloadSize,
		maxUncompressedSize: maxUncompressedSize,
		maxUnzippedItemSize: maxPayloadSize - len(footer) - len(header),
		maxZippedItemSize:   maxUncompressedSize - compression.CompressBound(len(footer)+len(header)),
		separator:           separator,
	}

	c.zipper = zlib.NewWriter(c.compressed)
	n, err := c.zipper.Write(header)
	c.uncompressedWritten += n

	return c, err
}

// checkItemSize checks that the item can fit in a payload. Worst case is used to
// determine the size of the item after compression meaning we could drop an item
// that could actually fit after compression. That said it is probably impossible
// to have a 2MB+ item that is valid for the backend.
func (c *Compressor) checkItemSize(data []byte) bool {
	maxEffectivePayloadSize := (c.maxPayloadSize - len(c.footer) - len(c.header))
	compressedWillFit := compression.CompressBound(len(data)) < c.maxZippedItemSize && compression.CompressBound(len(data)) < maxEffectivePayloadSize

	return len(data) < c.maxUnzippedItemSize && compressedWillFit
}

// hasRoomForItem checks if the current payload has enough room to store the given item
func (c *Compressor) hasRoomForItem(item []byte) bool {
	uncompressedDataSize := c.input.Len() + len(item)
	if !c.firstItem {
		uncompressedDataSize += len(c.separator)
	}
	return compression.CompressBound(uncompressedDataSize) <= c.remainingSpace() && c.uncompressedWritten+uncompressedDataSize <= c.maxUncompressedSize
}

// pack flushes the temporary uncompressed buffer input to the compression writer
func (c *Compressor) pack() error {
	expvarsTotalCycles.Add(1)
	tlmTotalCycles.Inc()
	n, err := c.input.WriteTo(c.zipper)
	if err != nil {
		return err
	}
	c.uncompressedWritten += int(n)
	c.zipper.Flush()
	c.input.Reset()
	return nil
}

func (c *Compressor) Write(data []byte) (int, error) {
	err := c.AddItem(data)
	return len(data), err
}

// AddItem will try to add the given item
func (c *Compressor) AddItem(data []byte) error {
	// check item size sanity
	if !c.checkItemSize(data) {
		return ErrItemTooBig
	}
	// check max repack cycles
	if c.repacks >= maxRepacks {
		return ErrPayloadFull
	}

	if !c.hasRoomForItem(data) {
		if c.input.Len() == 0 {
			return ErrPayloadFull
		}
		err := c.pack()
		if err != nil {
			return err
		}
		if !c.hasRoomForItem(data) {
			return ErrPayloadFull
		}
		c.repacks++
	}

	// Write the separator between items
	if c.firstItem {
		c.firstItem = false
	} else {
		c.input.Write(c.separator)
	}

	c.input.Write(data)
	return nil
}

// Close closes the Compressor, flushing any remaining uncompressed data
func (c *Compressor) Close() ([]byte, error) {
	// Flush remaining uncompressed data
	if c.input.Len() > 0 {
		n, err := c.input.WriteTo(c.zipper)
		c.uncompressedWritten += int(n)
		if err != nil {
			return nil, err
		}
	}
	// Add json footer
	n, err := c.zipper.Write(c.footer)
	c.uncompressedWritten += n
	if err != nil {
		return nil, err
	}
	// Add zlib footer and close
	err = c.zipper.Close()
	if err != nil {
		return nil, err
	}

	payload := make([]byte, c.compressed.Len())
	copy(payload, c.compressed.Bytes())

	expvarsTotalPayloads.Add(1)
	tlmTotalPayloads.Inc()
	expvarsBytesIn.Add(int64(c.uncompressedWritten))
	tlmBytesIn.Add(float64(c.uncompressedWritten))
	expvarsBytesOut.Add(int64(c.compressed.Len()))
	tlmBytesOut.Add(float64(c.compressed.Len()))

	return payload, nil
}

func (c *Compressor) remainingSpace() int {
	return c.maxPayloadSize - c.compressed.Len() - len(c.footer)
}
