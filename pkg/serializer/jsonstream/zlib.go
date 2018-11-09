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

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// Available is true if the code is compiled in
	Available = true
)

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

func zlibWorstCase(i int) int {
	return i
}

type chunk struct {
	input               *bytes.Buffer
	compressed          *bytes.Buffer
	zipper              *zlib.Writer
	header              []byte
	footer              []byte
	uncompressedWritten int // uncompressed bytes written
	firstItem           bool
}

func startChunk(header, footer []byte) (*chunk, error) {
	c := &chunk{
		header:     header,
		footer:     footer,
		input:      bytes.NewBuffer(make([]byte, 10*megaByte)),
		compressed: bytes.NewBuffer(make([]byte, 1*megaByte)),
		firstItem:  true,
	}

	c.zipper = zlib.NewWriter(c.compressed)
	_, err := c.zipper.Write(header)
	return c, err
}

// addItem will try to add
func (c *chunk) addItem(data []byte) error {
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

	if zlibWorstCase(toWrite) >= c.remainingSpace() {
		// Possibly reached maximum compressed size, compress and retry
		if c.input.Len() > 0 {
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

func (c *chunk) close() ([]byte, error) {
	// Flush remaining uncompressed data
	if c.input.Len() > 0 {
		_, err := c.input.WriteTo(c.zipper)
		if err != nil {
			return nil, err
		}
	}
	// Add json footer
	_, err := c.zipper.Write(c.footer)
	if err != nil {
		return nil, err
	}
	// Add zstd footer and close
	err = c.zipper.Close()
	if err != nil {
		return nil, err
	}

	return c.compressed.Bytes(), nil
}

func (c *chunk) remainingSpace() int {
	return maxPayloadSize - c.compressed.Len() - len(c.footer)
}

// Payloads serializes a metadata payload and sends it to the forwarder
func Payloads(m marshaler.StreamJSONMarshaler) (forwarder.Payloads, error) {
	var output forwarder.Payloads
	var i int
	itemCount := m.Len()

	chunk, err := startChunk(m.JSONHeader(), m.JSONFooter())
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

		switch chunk.addItem(json) {
		case errNeedSplit:
			// Need to split to a new payload
			payload, err := chunk.close()
			if err != nil {
				return output, err
			}
			output = append(output, &payload)
			chunk, err = startChunk(m.JSONHeader(), m.JSONFooter())
			if err != nil {
				return nil, err
			}
		case nil:
			// All good, continue to next item
			i++
			continue
		default:
			// Unexpected error, drop the item
			i++
			log.Warnf("Dropping an item: %s", err)
			continue
		}
	}

	return output, nil
}
