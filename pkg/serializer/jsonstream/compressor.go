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
	"sync"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	megaByte            = 1024 * 1024
	maxPayloadSize      = 2*megaByte + megaByte/2 // `2.5*megaByte` won't work with strong typing
	maxUncompressedSize = 45 * megaByte
	maxRepacks          = 40 // CPU time vs tighter payload tradeoff
)

var (
	errPayloadFull = errors.New("reached maximum payload size")
	errTooBig      = errors.New("item alone exceeds maximum payload size")
)

var jsonSeparator = []byte(",")

// inputBufferPool is an object pool of inputBuffers
// buffer is pre-allocated at creation to avoid heap trash.
// As hasRoomForItem is conservative, input buffer should not
// grow bigger than the compressed payload size.
var inputBufferPool = sync.Pool{
	New: func() interface{} {
		return bytes.NewBuffer(make([]byte, 0, maxPayloadSize))
	},
}

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
}

func newCompressor(header, footer []byte) (*compressor, error) {
	c := &compressor{
		header:              header,
		footer:              footer,
		compressed:          bytes.NewBuffer(make([]byte, 0, 1*megaByte)),
		firstItem:           true,
		maxUnzippedItemSize: maxPayloadSize - len(footer) - len(header),
		maxZippedItemSize:   maxUncompressedSize - compression.CompressBound(len(footer)+len(header)),
	}

	c.input = inputBufferPool.Get().(*bytes.Buffer)
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
	return compression.CompressBound(uncompressedDataSize) <= c.remainingSpace() && c.uncompressedWritten+uncompressedDataSize <= maxUncompressedSize
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

	c.input.Reset()
	inputBufferPool.Put(c.input)

	expvarsTotalPayloads.Add(1)
	expvarsBytesIn.Add(int64(c.uncompressedWritten))
	expvarsBytesOut.Add(int64(c.compressed.Len()))

	return c.compressed.Bytes(), nil
}

func (c *compressor) remainingSpace() int {
	return maxPayloadSize - c.compressed.Len() - len(c.footer)
}

// Payloads serializes a metadata payload and sends it to the forwarder
func Payloads(m marshaler.StreamJSONMarshaler) (forwarder.Payloads, error) {
	var output forwarder.Payloads
	var i int
	itemCount := m.Len()
	expvarsTotalCalls.Add(1)

	compressor, err := newCompressor(m.JSONHeader(), m.JSONFooter())
	if err != nil {
		return nil, err
	}

	for i < itemCount {
		json, err := m.JSONItem(i)
		if err != nil {
			log.Warnf("error marshalling an item, skipping: %s", err)
			i++
			continue
		}

		switch compressor.addItem(json) {
		case errPayloadFull:
			// payload is full, we need to create a new one
			payload, err := compressor.close()
			if err != nil {
				return output, err
			}
			output = append(output, &payload)
			compressor, err = newCompressor(m.JSONHeader(), m.JSONFooter())
			if err != nil {
				return nil, err
			}
		case nil:
			// All good, continue to next item
			i++
			expvarsTotalItems.Add(1)
			continue
		default:
			// Unexpected error, drop the item
			i++
			log.Warnf("Dropping an item, %s: %s", m.DescribeItem(i), err)
			expvarsItemDrops.Add(1)
			continue
		}
	}

	// Close last payload
	payload, err := compressor.close()
	if err != nil {
		return output, err
	}
	output = append(output, &payload)

	return output, nil
}
