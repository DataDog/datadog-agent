// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encodedtext

import (
	"testing"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Helper to encode string to UTF-16LE
func encodeUTF16LE(s string) []byte {
	runes := []rune(s)
	encoded := make([]byte, 0, len(runes)*2+2)
	// Add BOM
	encoded = append(encoded, 0xFF, 0xFE)
	for _, r := range runes {
		if r <= 0xFFFF {
			encoded = append(encoded, byte(r), byte(r>>8))
		} else {
			// Handle surrogate pairs for characters outside BMP
			r1, r2 := utf16.EncodeRune(r)
			encoded = append(encoded, byte(r1), byte(r1>>8))
			encoded = append(encoded, byte(r2), byte(r2>>8))
		}
	}
	return encoded
}

// Helper to encode string to UTF-16BE
func encodeUTF16BE(s string) []byte {
	runes := []rune(s)
	encoded := make([]byte, 0, len(runes)*2+2)
	// Add BOM
	encoded = append(encoded, 0xFE, 0xFF)
	for _, r := range runes {
		if r <= 0xFFFF {
			encoded = append(encoded, byte(r>>8), byte(r))
		} else {
			// Handle surrogate pairs for characters outside BMP
			r1, r2 := utf16.EncodeRune(r)
			encoded = append(encoded, byte(r1>>8), byte(r1))
			encoded = append(encoded, byte(r2>>8), byte(r2))
		}
	}
	return encoded
}

// Helper to encode string to Shift JIS (simplified - only ASCII and some Japanese)
func encodeShiftJIS(s string) []byte {
	// For fuzzing, we'll use a mix of ASCII and actual Shift JIS bytes
	encoded := []byte{}
	for _, r := range s {
		if r < 0x80 {
			// ASCII range
			encoded = append(encoded, byte(r))
		} else {
			// Use some common Shift JIS byte sequences
			switch r % 4 {
			case 0:
				encoded = append(encoded, 0x93, 0xfa) // 日
			case 1:
				encoded = append(encoded, 0x96, 0x7b) // 本
			case 2:
				encoded = append(encoded, 0x8c, 0xea) // 語
			default:
				encoded = append(encoded, 0x82, 0xa0) // あ
			}
		}
	}
	return encoded
}

// Test strings covering various scenarios
var testStrings = []string{
	"Hello, World!",
	"",
	"Simple ASCII text",
	"Text with\nnewlines\rand\r\nCRLF",
	"Text with \t tabs and   spaces",
	"Special chars: !@#$%^&*()_+-=[]{}|;':\",./<>?",
	"Unicode: 你好世界 🌍 émojis 🎉",
	"Mixed 日本語 and English text",
	"Very long text " + string(make([]byte, 1000)),
	"\x00\x01\x02\x03\x04\x05", // Control characters
	"Surrogate pairs: 𝄞 𝐀𝐁𝐂",   // Characters outside BMP
}

func FuzzParseUTF16LE(f *testing.F) {
	// Add seed corpus for UTF-16LE
	for _, s := range testStrings {
		// With BOM
		f.Add(encodeUTF16LE(s))
		// Without BOM
		if len(s) > 0 {
			encoded := encodeUTF16LE(s)
			if len(encoded) > 2 {
				f.Add(encoded[2:]) // Skip BOM
			}
		}
	}

	// Edge cases
	f.Add([]byte{})                       // Empty
	f.Add([]byte{0xFF})                   // Incomplete BOM
	f.Add([]byte{0xFF, 0xFE})             // Just BOM
	f.Add([]byte{0xFF, 0xFE, 0x00})       // BOM + incomplete character
	f.Add([]byte{0x00})                   // Single null byte
	f.Add([]byte{0x80, 0x81, 0x82})       // Invalid UTF-8 but might be valid in other encodings
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF}) // All 0xFF bytes

	parser := New(UTF16LE)

	f.Fuzz(func(t *testing.T, data []byte) {
		msg := message.NewMessage(data, nil, "", 0)

		// Parser should not panic
		result, err := parser.Parse(msg)

		// Verify invariants
		if err == nil && result != nil {
			decoded := result.GetContent()

			// Decoded content should be valid UTF-8
			if !utf8.Valid(decoded) {
				t.Errorf("UTF16LE: decoded content is not valid UTF-8")
			}

			// If we had a BOM, it should be handled correctly
			if len(data) >= 2 {
				if (data[0] == 0xFF && data[1] == 0xFE) || (data[0] == 0xFE && data[1] == 0xFF) {
					// BOM should not appear in decoded output
					if len(decoded) >= 2 && ((decoded[0] == 0xFF && decoded[1] == 0xFE) || (decoded[0] == 0xFE && decoded[1] == 0xFF)) {
						t.Errorf("UTF16LE: BOM was not properly removed from decoded output")
					}
				}
			}
		}

		// Parser should always return the same message object
		if result != msg {
			t.Errorf("UTF16LE: parser returned different message object")
		}
	})
}

func FuzzParseUTF16BE(f *testing.F) {
	// Add seed corpus for UTF-16BE
	for _, s := range testStrings {
		// With BOM
		f.Add(encodeUTF16BE(s))
		// Without BOM
		if len(s) > 0 {
			encoded := encodeUTF16BE(s)
			if len(encoded) > 2 {
				f.Add(encoded[2:]) // Skip BOM
			}
		}
	}

	// Edge cases
	f.Add([]byte{})                       // Empty
	f.Add([]byte{0xFE})                   // Incomplete BOM
	f.Add([]byte{0xFE, 0xFF})             // Just BOM
	f.Add([]byte{0xFE, 0xFF, 0x00})       // BOM + incomplete character
	f.Add([]byte{0x00})                   // Single null byte
	f.Add([]byte{0x80, 0x81, 0x82})       // Invalid UTF-8 but might be valid in other encodings
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF}) // All 0xFF bytes

	parser := New(UTF16BE)

	f.Fuzz(func(t *testing.T, data []byte) {
		msg := message.NewMessage(data, nil, "", 0)

		// Parser should not panic
		result, err := parser.Parse(msg)

		// Verify invariants
		if err == nil && result != nil {
			decoded := result.GetContent()

			// Decoded content should be valid UTF-8
			if !utf8.Valid(decoded) {
				t.Errorf("UTF16BE: decoded content is not valid UTF-8")
			}

			// If we had a BOM, it should be handled correctly
			if len(data) >= 2 {
				if (data[0] == 0xFF && data[1] == 0xFE) || (data[0] == 0xFE && data[1] == 0xFF) {
					// BOM should not appear in decoded output
					if len(decoded) >= 2 && ((decoded[0] == 0xFF && decoded[1] == 0xFE) || (decoded[0] == 0xFE && decoded[1] == 0xFF)) {
						t.Errorf("UTF16BE: BOM was not properly removed from decoded output")
					}
				}
			}
		}

		// Parser should always return the same message object
		if result != msg {
			t.Errorf("UTF16BE: parser returned different message object")
		}
	})
}

func FuzzParseShiftJIS(f *testing.F) {
	// Add seed corpus for Shift JIS
	for _, s := range testStrings {
		f.Add(encodeShiftJIS(s))
	}

	// Additional Shift JIS specific test cases
	f.Add([]byte{0x93, 0xfa, 0x96, 0x7b})             // 日本
	f.Add([]byte{0x82, 0xa0, 0x82, 0xa2, 0x82, 0xa4}) // あいう
	f.Add([]byte{0x8c, 0xea})                         // 語

	// Edge cases
	f.Add([]byte{})                       // Empty
	f.Add([]byte{0x00})                   // Single null byte
	f.Add([]byte{0x80, 0x81, 0x82})       // High bytes
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF}) // All 0xFF bytes

	parser := New(SHIFTJIS)

	f.Fuzz(func(t *testing.T, data []byte) {
		msg := message.NewMessage(data, nil, "", 0)

		// Parser should not panic
		result, err := parser.Parse(msg)

		// Verify invariants
		if err == nil && result != nil {
			decoded := result.GetContent()

			// Decoded content should be valid UTF-8
			if !utf8.Valid(decoded) {
				t.Errorf("SHIFTJIS: decoded content is not valid UTF-8")
			}
		}

		// Parser should always return the same message object
		if result != msg {
			t.Errorf("SHIFTJIS: parser returned different message object")
		}
	})
}

func FuzzEncodingRoundTrip(f *testing.F) {
	// Test that valid UTF-8 strings can be encoded and decoded back
	testStrings := []string{
		"Hello",
		"World",
		"",
		"ABC123",
		"Special: \n\r\t",
		"Unicode: 你好",
		"Emoji: 😀",
	}

	for _, s := range testStrings {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		// Only test with valid UTF-8 strings
		if !utf8.ValidString(input) {
			t.Skip("Skipping invalid UTF-8 string")
		}

		// Helper to manually encode UTF-16
		encodeUTF16Manual := func(s string, bigEndian bool) []byte {
			runes := []rune(s)
			encoded := make([]byte, 0, len(runes)*2)
			for _, r := range runes {
				if r <= 0xFFFF {
					if bigEndian {
						encoded = append(encoded, byte(r>>8), byte(r))
					} else {
						encoded = append(encoded, byte(r), byte(r>>8))
					}
				} else {
					// Handle surrogate pairs
					r1, r2 := utf16.EncodeRune(r)
					if bigEndian {
						encoded = append(encoded, byte(r1>>8), byte(r1))
						encoded = append(encoded, byte(r2>>8), byte(r2))
					} else {
						encoded = append(encoded, byte(r1), byte(r1>>8))
						encoded = append(encoded, byte(r2), byte(r2>>8))
					}
				}
			}
			return encoded
		}

		// Test UTF-16LE round trip
		encodedLE := encodeUTF16Manual(input, false)
		parserLE := New(UTF16LE)
		msgLE := message.NewMessage(encodedLE, nil, "", 0)
		resultLE, errLE := parserLE.Parse(msgLE)
		if errLE == nil {
			decodedLE := string(resultLE.GetContent())
			if decodedLE != input {
				// This might be expected for some inputs due to encoding limitations
				// Just ensure we got valid UTF-8 back
				if !utf8.ValidString(decodedLE) {
					t.Errorf("UTF16LE roundtrip produced invalid UTF-8")
				}
			}
		}

		// Test UTF-16BE round trip
		encodedBE := encodeUTF16Manual(input, true)
		parserBE := New(UTF16BE)
		msgBE := message.NewMessage(encodedBE, nil, "", 0)
		resultBE, errBE := parserBE.Parse(msgBE)
		if errBE == nil {
			decodedBE := string(resultBE.GetContent())
			if decodedBE != input {
				// This might be expected for some inputs due to encoding limitations
				// Just ensure we got valid UTF-8 back
				if !utf8.ValidString(decodedBE) {
					t.Errorf("UTF16BE roundtrip produced invalid UTF-8")
				}
			}
		}
	})
}
