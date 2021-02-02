// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parser

import (
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// NoopParser is the default parser and does nothing
var NoopParser *noopParser

// Encoding is our internal type for supported encoding for the DecodingParser
type Encoding int

const (
	// UTF16LE UTF16 little endian, most common (windows)
	UTF16LE = iota
	// UTF16BE UTF16 big endian
	UTF16BE
)

// Parser parse messages
type Parser interface {
	// It returns 1. raw message, 2. severity, 3. timestamp, 4. partial, 5. error
	Parse([]byte) ([]byte, string, string, bool, error)
	SupportsPartialLine() bool
}

type noopParser struct{}

// Parse does nothing for NoopParser
func (p *noopParser) Parse(msg []byte) ([]byte, string, string, bool, error) {
	return msg, "", "", false, nil
}

func (p *noopParser) SupportsPartialLine() bool {
	return false
}

// DecodingParser a generic decoding Parser
type DecodingParser struct {
	decoder *encoding.Decoder
}

// Parse parses the incoming message with the decoder
func (p *DecodingParser) Parse(msg []byte) ([]byte, string, string, bool, error) {
	decoded, _, err := transform.Bytes(p.decoder, msg)
	return decoded, "", "", false, err
}

// SupportsPartialLine returns false as it does not support partial lines
func (p *DecodingParser) SupportsPartialLine() bool {
	return false
}

// NewDecodingParser build a new DecodingParser
func NewDecodingParser(e Encoding) *DecodingParser {
	p := &DecodingParser{}
	var enc encoding.Encoding
	switch e {
	case UTF16LE:
		enc = unicode.UTF16(unicode.LittleEndian, unicode.UseBOM)
	case UTF16BE:
		enc = unicode.UTF16(unicode.BigEndian, unicode.UseBOM)
	}
	p.decoder = enc.NewDecoder()
	return p
}
