// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

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
	// UTF16LE UTF16 little endian, most commong (windows)
	UTF16LE = iota
	// UTF16BE UTF16 big endian
	UTF16BE
)

// Parser parse messages
type Parser interface {
	Parse([]byte) ([]byte, string, string, error)
}

type noopParser struct {
	Parser
}

// Parse does nothing for NoopParser
func (p *noopParser) Parse(msg []byte) ([]byte, string, string, error) {
	return msg, "", "", nil
}

// DecodingParser a generic decoding Parser
type DecodingParser struct {
	decoder *encoding.Decoder
	Parser
}

// Parse does nothing for NoopParser
func (p *DecodingParser) Parse(msg []byte) ([]byte, string, string, error) {
	decoded, _, err := transform.Bytes(p.decoder, msg)
	return decoded, "", "", err
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
