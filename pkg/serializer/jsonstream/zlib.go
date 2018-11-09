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
	expvars.Set("TotalItem", &expvarsTotalItems)
	expvars.Set("TotalPayloads", &expvarsTotalPayloads)
	expvars.Set("TotalCompressCycles", &expvarsTotalCycles)
	expvars.Set("ItemDrops", &expvarsItemDrops)
	expvars.Set("BytesIn", &expvarsBytesIn)
	expvars.Set("BytesOut", &expvarsBytesOut)
}

// the backend accepts payloads up to 3MB/50MB, but being conservative is okay
var (
	megaByte            = 1024 * 1024
	maxPayloadSize      = 2*megaByte + megaByte/2
	maxUncompressedSize = 45 * megaByte
)

var (
	errNeedSplit = errors.New("reached maximum payload size, need to split")
	errTooBig    = errors.New("item alone exceeds maximum payload size")
)

var jsonSeparator = []byte(",")

type compressor struct {
	input               *bytes.Buffer
	compressed          *bytes.Buffer
	zipper              *zlib.Writer
	header              []byte
	footer              []byte
	uncompressedWritten int // uncompressed bytes written
	firstItem           bool
}

func newCompressor(header, footer []byte) (*compressor, error) {
	c := &compressor{
		header:     header,
		footer:     footer,
		input:      bytes.NewBuffer(make([]byte, 0, 10*megaByte)),
		compressed: bytes.NewBuffer(make([]byte, 0, 1*megaByte)),
		firstItem:  true,
	}

	c.zipper = zlib.NewWriter(c.compressed)
	_, err := c.zipper.Write(header)
	return c, err
}

// addItem will try to add
func (c *compressor) addItem(data []byte) error {
	toWrite := c.input.Len() + len(data) + len(c.footer)
	if !c.firstItem {
		toWrite += len(jsonSeparator)
	}
	if c.uncompressedWritten+toWrite >= maxUncompressedSize {
		// Reached maximum uncompressed size
		if len(c.header)+len(data)+len(c.footer) >= maxUncompressedSize {
			// Item alone is too big for max uncompressed size
			return errTooBig
		}
		return errNeedSplit
	}

	if compression.CompressBound(toWrite) >= c.remainingSpace() {
		// Possibly reached maximum compressed size, compress and retry
		if c.input.Len() > 0 {
			expvarsTotalCycles.Add(1)
			n, err := c.input.WriteTo(c.zipper)
			if err != nil {
				return err
			}
			c.uncompressedWritten += int(n)
			c.zipper.Flush()
			c.input.Reset()
			return c.addItem(data)
		}
		return errNeedSplit
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
	// Add zstd footer and close
	err = c.zipper.Close()
	if err != nil {
		return nil, err
	}

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
		case errNeedSplit:
			// Need to split to a new payload
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
			log.Warnf("Dropping an item: %s", err)
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
