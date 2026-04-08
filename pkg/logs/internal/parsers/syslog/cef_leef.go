// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"bytes"
	"strings"
)

// SIEMHeader holds the normalized header fields common to both CEF and LEEF.
type SIEMHeader struct {
	Format        string // "CEF" or "LEEF"
	Version       string // e.g. "0", "1" for CEF; "1.0", "2.0" for LEEF
	DeviceVendor  string
	DeviceProduct string
	DeviceVersion string
	EventID       string // Signature ID (CEF) or Event ID (LEEF)
	Name          string // CEF only; empty for LEEF
	Severity      string // CEF header severity; empty for LEEF
}

var (
	prefixCEF  = []byte("CEF:")
	prefixLEEF = []byte("LEEF:")
)

// ParseCEFLEEF detects whether msg starts with a CEF or LEEF header and, if
// so, parses the pipe-delimited header fields and the extension key-value
// pairs. Returns ok=false if the message is not CEF/LEEF or is malformed.
func ParseCEFLEEF(msg []byte) (header SIEMHeader, extension map[string]string, rawExtension []byte, ok bool) {
	switch {
	case bytes.HasPrefix(msg, prefixCEF):
		return parseCEF(msg)
	case bytes.HasPrefix(msg, prefixLEEF):
		return parseLEEF(msg)
	default:
		return SIEMHeader{}, nil, nil, false
	}
}

// parseCEF parses: CEF:Version|Vendor|Product|DevVersion|SigID|Name|Severity|Extension
// Requires exactly 7 pipe-delimited fields after the "CEF:" prefix.
func parseCEF(msg []byte) (SIEMHeader, map[string]string, []byte, bool) {
	rest := msg[len(prefixCEF):]

	fields, ext, ok := splitHeaderPipes(rest, 7)
	if !ok {
		return SIEMHeader{}, nil, nil, false
	}

	header := SIEMHeader{
		Format:        "CEF",
		Version:       unescapeCEFHeader(fields[0]),
		DeviceVendor:  unescapeCEFHeader(fields[1]),
		DeviceProduct: unescapeCEFHeader(fields[2]),
		DeviceVersion: unescapeCEFHeader(fields[3]),
		EventID:       unescapeCEFHeader(fields[4]),
		Name:          unescapeCEFHeader(fields[5]),
		Severity:      unescapeCEFHeader(fields[6]),
	}

	extension := parseCEFExtension(ext)
	return header, extension, ext, true
}

// parseLEEF parses LEEF 1.0 and 2.0 messages.
//
// LEEF 1.0: LEEF:1.0|Vendor|Product|Version|EventID|<tab-delimited extension>
// LEEF 2.0: LEEF:2.0|Vendor|Product|Version|EventID|Delimiter|<extension>
func parseLEEF(msg []byte) (SIEMHeader, map[string]string, []byte, bool) {
	rest := msg[len(prefixLEEF):]

	pipeIdx := bytes.IndexByte(rest, '|')
	if pipeIdx < 0 {
		return SIEMHeader{}, nil, nil, false
	}
	version := string(rest[:pipeIdx])

	var delimiter byte = '\t' // LEEF 1.0 default
	var fields []string
	var ext []byte
	var ok bool

	switch {
	case strings.HasPrefix(version, "2."):
		// LEEF 2.0: 6 pipe-delimited fields (version + vendor + product + devVersion + eventID + delimiter)
		fields, ext, ok = splitHeaderPipes(rest, 6)
		if !ok {
			return SIEMHeader{}, nil, nil, false
		}
		if d, valid := parseLEEFDelimiter(fields[5]); valid {
			delimiter = d
		}
	default:
		// LEEF 1.0 (or unknown): 5 pipe-delimited fields
		fields, ext, ok = splitHeaderPipes(rest, 5)
		if !ok {
			return SIEMHeader{}, nil, nil, false
		}
	}

	header := SIEMHeader{
		Format:        "LEEF",
		Version:       fields[0],
		DeviceVendor:  fields[1],
		DeviceProduct: fields[2],
		DeviceVersion: fields[3],
		EventID:       fields[4],
	}

	extension := parseLEEFExtension(ext, delimiter)
	return header, extension, ext, true
}

// splitHeaderPipes splits b into exactly n pipe-delimited header fields.
// The content after the last pipe is returned as the extension.
// Escaped pipes (\|) inside field values are handled correctly.
func splitHeaderPipes(b []byte, n int) (fields []string, extension []byte, ok bool) {
	fields = make([]string, 0, n)
	start := 0
	for i := 0; i < len(b); i++ {
		if b[i] == '\\' {
			i++ // skip escaped character
			continue
		}
		if b[i] == '|' {
			fields = append(fields, string(b[start:i]))
			start = i + 1
			if len(fields) == n {
				return fields, b[start:], true
			}
		}
	}
	return nil, nil, false
}

// parseCEFExtension parses CEF extension key-value pairs where spaces
// separate pairs but values may also contain spaces.
//
// Key boundaries are identified by the pattern <SP><keyChars>= where
// keyChars is [a-zA-Z0-9_]+. The value runs from the = until the space
// before the next key boundary (or end of input for the last pair).
func parseCEFExtension(ext []byte) map[string]string {
	if len(ext) == 0 {
		return nil
	}

	s := string(ext)

	// Phase 1: find all key-boundary positions.
	// A key boundary is at index i where s[i-1]==' ' (or i==0) and
	// s[i..j-1] matches [a-zA-Z0-9_]+ followed by '='.
	type keySpan struct {
		keyStart int // index of first key char
		eqPos    int // index of '='
	}
	var boundaries []keySpan

	i := 0
	for i < len(s) {
		// A key can start at position 0, or after a space.
		if i > 0 && s[i-1] != ' ' {
			i++
			continue
		}
		// Scan forward for [a-zA-Z0-9_]+ followed by '='
		j := i
		for j < len(s) && isKeyChar(s[j]) {
			j++
		}
		if j > i && j < len(s) && s[j] == '=' {
			// Check that the '=' is not escaped
			if j > 0 && s[j-1] == '\\' {
				i = j + 1
				continue
			}
			boundaries = append(boundaries, keySpan{keyStart: i, eqPos: j})
			i = j + 1
		} else {
			i++
		}
	}

	if len(boundaries) == 0 {
		return nil
	}

	// Phase 2: extract key-value pairs using the boundaries.
	result := make(map[string]string, len(boundaries))
	for idx, b := range boundaries {
		key := s[b.keyStart:b.eqPos]
		var value string
		valStart := b.eqPos + 1
		if idx+1 < len(boundaries) {
			// Value runs up to the space before the next key boundary.
			// The space immediately before the next key is the delimiter.
			valEnd := boundaries[idx+1].keyStart - 1
			if valEnd < valStart {
				valEnd = valStart
			}
			value = s[valStart:valEnd]
		} else {
			// Last pair: value runs to end, with trailing spaces stripped.
			value = strings.TrimRight(s[valStart:], " ")
		}
		result[key] = unescapeCEFValue(value)
	}
	return result
}

// parseLEEFExtension parses LEEF extension key-value pairs separated by
// the given delimiter. Within each pair, the first '=' splits key from value.
func parseLEEFExtension(ext []byte, delimiter byte) map[string]string {
	if len(ext) == 0 {
		return nil
	}

	s := string(ext)
	result := make(map[string]string)

	for _, token := range splitOnByte(s, delimiter) {
		if len(token) == 0 {
			continue
		}
		eqIdx := strings.IndexByte(token, '=')
		if eqIdx < 0 {
			continue
		}
		key := token[:eqIdx]
		value := token[eqIdx+1:]
		if key != "" {
			result[key] = value
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// splitOnByte splits s on every occurrence of sep, returning all substrings
// including empty ones. Unlike strings.Split, this avoids allocating when
// the separator is a single byte.
func splitOnByte(s string, sep byte) []string {
	n := strings.Count(s, string(sep)) + 1
	parts := make([]string, 0, n)
	for {
		idx := strings.IndexByte(s, sep)
		if idx < 0 {
			parts = append(parts, s)
			break
		}
		parts = append(parts, s[:idx])
		s = s[idx+1:]
	}
	return parts
}

// parseLEEFDelimiter parses the LEEF 2.0 delimiter field.
// Accepts a single literal character or hex notation (0xHH or xHH).
func parseLEEFDelimiter(field string) (byte, bool) {
	if len(field) == 0 {
		return 0, false
	}

	// Hex notation: "0x..." or "x..."
	var hexStr string
	switch {
	case strings.HasPrefix(field, "0x") || strings.HasPrefix(field, "0X"):
		hexStr = field[2:]
	case (field[0] == 'x' || field[0] == 'X') && len(field) > 1:
		hexStr = field[1:]
	}

	if hexStr != "" {
		var val byte
		for _, c := range []byte(hexStr) {
			var nibble byte
			switch {
			case c >= '0' && c <= '9':
				nibble = c - '0'
			case c >= 'a' && c <= 'f':
				nibble = c - 'a' + 10
			case c >= 'A' && c <= 'F':
				nibble = c - 'A' + 10
			default:
				return 0, false
			}
			val = val<<4 | nibble
		}
		return val, true
	}

	// Single literal character
	if len(field) == 1 {
		return field[0], true
	}
	return 0, false
}

// unescapeCEFHeader unescapes \\  and \| in CEF header field values.
func unescapeCEFHeader(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var buf strings.Builder
	buf.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			next := s[i+1]
			switch next {
			case '\\', '|':
				buf.WriteByte(next)
				i++
				continue
			}
		}
		buf.WriteByte(s[i])
	}
	return buf.String()
}

// unescapeCEFValue unescapes \\, \=, \n, and \r in CEF extension values.
func unescapeCEFValue(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var buf strings.Builder
	buf.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			next := s[i+1]
			switch next {
			case '\\':
				buf.WriteByte('\\')
				i++
				continue
			case '=':
				buf.WriteByte('=')
				i++
				continue
			case 'n':
				buf.WriteByte('\n')
				i++
				continue
			case 'r':
				buf.WriteByte('\r')
				i++
				continue
			}
		}
		buf.WriteByte(s[i])
	}
	return buf.String()
}

// isKeyChar returns true for characters valid in a CEF extension key: [a-zA-Z0-9_].
func isKeyChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// BuildSIEMFields converts a SIEMHeader and parsed extension into the map
// stored under the "siem" key in BasicStructuredContent.
func BuildSIEMFields(header SIEMHeader, extension map[string]string) map[string]interface{} {
	fields := map[string]interface{}{
		"format":         header.Format,
		"version":        header.Version,
		"device_vendor":  header.DeviceVendor,
		"device_product": header.DeviceProduct,
		"device_version": header.DeviceVersion,
		"event_id":       header.EventID,
	}
	if header.Name != "" {
		fields["name"] = header.Name
	}
	if header.Severity != "" {
		fields["severity"] = header.Severity
	}
	if len(extension) > 0 {
		fields["extension"] = extension
	}
	return fields
}
