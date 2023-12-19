// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package encodedtext parses plain text messages that are in encodings other than utf-8.
package encodedtext

import (
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Encoding specifies the encoding which should be decoded by the DecodingParser
type Encoding int

const (
	// UTF16LE UTF16 little endian, most common (windows)
	UTF16LE Encoding = iota
	// UTF16BE UTF16 big endian
	UTF16BE
	// SHIFTJIS Shift JIS (Japanese)
	SHIFTJIS
)

type encodedText struct {
	decoder *encoding.Decoder
}

// Parse implements Parser#Parse
func (p *encodedText) Parse(msg *message.Message) (*message.Message, error) {
	decoded, _, err := transform.Bytes(p.decoder, msg.GetContent())
	msg.SetContent(decoded)
	return msg, err
}

// SupportsPartialLine implements Parser#SupportsPartialLine
func (p *encodedText) SupportsPartialLine() bool {
	return false
}

// New builds a new parser for decoding encoded logfiles.  It treats each input
// message as entirely content, in the given encoding.  No timetamp or other
// metadata are returned.
func New(e Encoding) parsers.Parser {
	p := &encodedText{}
	var enc encoding.Encoding
	switch e {
	case UTF16LE:
		enc = unicode.UTF16(unicode.LittleEndian, unicode.UseBOM)
	case UTF16BE:
		enc = unicode.UTF16(unicode.BigEndian, unicode.UseBOM)
	case SHIFTJIS:
		enc = japanese.ShiftJIS
	}
	p.decoder = enc.NewDecoder()
	return p
}
