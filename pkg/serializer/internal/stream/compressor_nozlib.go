// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-2020 Datadog, Inc.

//go:build !zlib

package stream

import (
	"bytes"
	"errors"
	"fmt"
)

const (
	// Available is true if the code is compiled in
	Available = false
)

var (
	// ErrPayloadFull is returned when the payload buffer is full
	ErrPayloadFull = errors.New("reached maximum payload size")

	// ErrItemTooBig is returned when a item alone exceeds maximum payload size
	ErrItemTooBig = errors.New("item alone exceeds maximum payload size")
)

// Compressor is not implemented
type Compressor struct{}

// NewCompressor not implemented
func NewCompressor(input, output *bytes.Buffer, maxPayloadSize, maxUncompressedSize int, header, footer []byte, separator []byte) (*Compressor, error) {
	return nil, fmt.Errorf("not implemented")
}

// AddItem not implemented
func (c *Compressor) AddItem(data []byte) error {
	return fmt.Errorf("not implemented")
}

// Close not implemented
func (c *Compressor) Close() ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}
