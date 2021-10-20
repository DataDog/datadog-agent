package parser

import (
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// Encoding specifies the encoding which should be decoded by the DecodingParser
type Encoding int

const (
	// UTF16LE UTF16 little endian, most common (windows)
	UTF16LE Encoding = iota
	// UTF16BE UTF16 big endian
	UTF16BE
)

// EncodedText a parser for decoding encoded logfiles.  It treats each input
// message as entirely content, in the given encoding.  No timetamp or other
// metadata are returned.
type EncodedText struct {
	decoder *encoding.Decoder
}

// Parse implements Parser#Parse
func (p *EncodedText) Parse(msg []byte) ([]byte, string, string, bool, error) {
	decoded, _, err := transform.Bytes(p.decoder, msg)
	return decoded, "", "", false, err
}

// SupportsPartialLine implements Parser#SupportsPartialLine
func (p *EncodedText) SupportsPartialLine() bool {
	return false
}

// NewEncodedText builds a new DecodingParser.
func NewEncodedText(e Encoding) *EncodedText {
	p := &EncodedText{}
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
