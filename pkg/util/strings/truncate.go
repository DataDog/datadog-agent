// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package strings contains utilities for working with strings in Go
package strings

const (
	utf8MultiByteFlag      = 0b10000000
	utf8MultiByteStartFlag = 0b11000000
	utf8TwoByteFlag        = 0b11000000
	utf8TwoByteMask        = 0b11100000
	utf8ThreeByteFlag      = 0b11100000
	utf8ThreeByteMask      = 0b11110000
	utf8FourByteFlag       = 0b11110000
	utf8FourByteMask       = 0b11111000
)

// TruncateUTF8 truncates the given string to make sure it uses less than limit bytes.
// If the last code point is an utf8 code point that would be splitten, it removes it
// entirely to make sure the resulting string is not broken.
//
// This function may still split characters that are comprised of more than one valid
// utf8 code point, like many emoji, however the resulting string will still be valid utf8.
// Example: The "Flag: United States emoji" is a flag sequence combining
//
//	🇺 Regional Indicator Symbol Letter U
//	🇸 Regional Indicator Symbol Letter S.
//
// via wikipedia https://en.wikipedia.org/wiki/UTF-8
//
// Code point ↔ UTF-8 conversion
//
//	First code point | Last code point | Byte 1   | Byte 2   | Byte 3   | Byte 4   |
//	U+0000           | U+007F          | 0xxxxxxx |          |          |          |
//	U+0080           | U+07FF          | 110xxxxx | 10xxxxxx |          |          |
//	U+0800           | U+FFFF          | 1110xxxx | 10xxxxxx | 10xxxxxx |          |
//	U+10000          | U+10FFFF        | 11110xxx | 10xxxxxx | 10xxxxxx | 10xxxxxx |
func TruncateUTF8(s string, limit int) string {
	if len(s) <= limit {
		return s
	} else if limit == 0 {
		return ""
	}

	// truncate to byte limit
	s = s[:limit]

	// check for trailing invalid bytes

	if s[limit-1]&utf8MultiByteFlag == 0 {
		// last byte is a valid single byte character
		return s
	} else if s[limit-1]&utf8MultiByteStartFlag == utf8MultiByteStartFlag {
		// last byte is start of a new multi-byte character (n bytes cut off)
		return s[:limit-1]
	} else if s[limit-2]&utf8TwoByteMask == utf8TwoByteFlag {
		// last two bytes are a valid two byte character
		return s
	} else if s[limit-2]&utf8ThreeByteMask == utf8ThreeByteFlag {
		// last two bytes are part of a three byte character (last byte cut off)
		return s[:limit-2]
	} else if s[limit-3]&utf8ThreeByteMask == utf8ThreeByteFlag {
		// last three bytes are a valid three byte character
		return s
	} else if s[limit-3]&utf8FourByteMask == utf8FourByteFlag {
		// last three bytes are part of a four byte character (last byte cut off)
		return s[:limit-3]
	}

	// last character is valid
	return s
}
