// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

//+build zstd

package jsonstream

import (
	"bytes"

	"github.com/DataDog/zstd"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

const (
	// Available is true if the code is compiled in
	Available = true
)

// the backend accepts payloads up to 3MB/50MB, but being conservative is okay
var maxPayloadSize = 2 * 1024 * 1024
var maxUncompressedSize = 40 * 1024 * 1024

type chunk struct {
	compressed bytes.Buffer
	writer     *zstd.Writer
}

func startChunk(header []byte) (*chunk, error) {
	c := &chunk{}
	c.writer = zstd.NewWriter(c.compressed)
	c.writer.Write(header)
	return c
}

func (c *chunk) close(footer []byte) ([]byte, error) {
	_, err := c.writer.Write(fooler)
	if err != nil {
		return nil, err
	}
	err := c.writer.Close()
	if err != nil {
		return nil, err
	}

	return c.compressed.Bytes(), nil
}

func (c *chunk) remainingSpace() int {
	return maxPayloadSize - c.compressed.Len()
}

// Payloads serializes a metadata payload and sends it to the forwarder
func Payloads(m marshaler.StreamMarshaler) (forwarder.Payloads, error) {
	var output forwarder.Payloads

	chunk, err := startChunk(m.MarshalHeaders())
	if err != nil {
		return nil, err
	}

	for item := range m.Items() {
		var inputBuffer bytes.Buffer

	}

	return output, nil
}
