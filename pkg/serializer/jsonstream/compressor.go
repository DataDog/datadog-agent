// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

//+build zlib

package jsonstream

import (
	"bytes"
	"compress/zlib"
	"errors"
	"expvar"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

const (
	// Available is true if the code is compiled in
	Available = true
)

var (
	expvars              = expvar.NewMap("jsonstream")
	expvarsTotalCalls    = expvar.Int{}
	expvarsTotalItems    = expvar.Int{}
	expvarsTotalPayloads = expvar.Int{}
	expvarsTotalCycles   = expvar.Int{}
	expvarsItemDrops     = expvar.Int{}
	expvarsBytesIn       = expvar.Int{}
	expvarsBytesOut      = expvar.Int{}
)

func init() {
	expvars.Set("TotalCalls", &expvarsTotalCalls)
	expvars.Set("TotalItems", &expvarsTotalItems)
	expvars.Set("TotalPayloads", &expvarsTotalPayloads)
	expvars.Set("TotalCompressCycles", &expvarsTotalCycles)
	expvars.Set("ItemDrops", &expvarsItemDrops)
	expvars.Set("BytesIn", &expvarsBytesIn)
	expvars.Set("BytesOut", &expvarsBytesOut)
}

// the backend accepts payloads up to 3MB/50MB, but being conservative is okay
var (
	maxRepacks = 40 // CPU time vs tighter payload tradeoff
)

var (
	errPayloadFull = errors.New("reached maximum payload size")
	errTooBig      = errors.New("item alone exceeds maximum payload size")
)

var jsonSeparator = []byte(",")

// compressor is in charge of compressing items for a single payload
type compressor struct {
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
}

func newCompressor(input, output *bytes.Buffer, header, footer []byte) (*compressor, error) {
	maxPayloadSize := config.Datadog.GetInt("serializer_max_payload_size")
	maxUncompressedSize := config.Datadog.GetInt("serializer_max_uncompressed_payload_size")
	c := &compressor{
		header:              header,
		footer:              footer,
		input:               input,
		compressed:          output,
		firstItem:           true,
		maxPayloadSize:      maxPayloadSize,
		maxUncompressedSize: maxUncompressedSize,
		maxUnzippedItemSize: maxPayloadSize - len(footer) - len(header),
		maxZippedItemSize:   maxUncompressedSize - compression.CompressBound(len(footer)+len(header)),
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
func (c *compressor) checkItemSize(data []byte) bool {
	return len(data) < c.maxUnzippedItemSize && compression.CompressBound(len(data)) < c.maxZippedItemSize
}

// hasRoomForItem checks if the current payload has enough room to store the given item
func (c *compressor) hasRoomForItem(item []byte) bool {
	uncompressedDataSize := c.input.Len() + len(item)
	if !c.firstItem {
		uncompressedDataSize += len(jsonSeparator)
	}
	return compression.CompressBound(uncompressedDataSize) <= c.remainingSpace() && c.uncompressedWritten+uncompressedDataSize <= c.maxUncompressedSize
}

// pack flushes the temporary uncompressed buffer input to the compression writer
func (c *compressor) pack() error {
	expvarsTotalCycles.Add(1)
	n, err := c.input.WriteTo(c.zipper)
	if err != nil {
		return err
	}
	c.uncompressedWritten += int(n)
	c.zipper.Flush()
	c.input.Reset()
	return nil
}

// addItem will try to add the given item
func (c *compressor) addItem(data []byte) error {
	// check item size sanity
	if !c.checkItemSize(data) {
		return errTooBig
	}
	// check max repack cycles
	if c.repacks >= maxRepacks {
		return errPayloadFull
	}

	if !c.hasRoomForItem(data) {
		if c.input.Len() == 0 {
			return errPayloadFull
		}
		err := c.pack()
		if err != nil {
			return err
		}
		if !c.hasRoomForItem(data) {
			return errPayloadFull
		}
		c.repacks++
	}

	// Write the separator between items
	if c.firstItem {
		c.firstItem = false
	} else {
		c.input.Write(jsonSeparator)
	}

	c.input.Write(data)
	return nil
}

func (c *compressor) close() ([]byte, error) {
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
	c.uncompressedWritten += int(n)
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
	expvarsBytesIn.Add(int64(c.uncompressedWritten))
	expvarsBytesOut.Add(int64(c.compressed.Len()))

	return payload, nil
}

func (c *compressor) remainingSpace() int {
	return c.maxPayloadSize - c.compressed.Len() - len(c.footer)
}
