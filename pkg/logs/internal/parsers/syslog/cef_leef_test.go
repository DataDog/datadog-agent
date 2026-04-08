// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// CEF header parsing
// ---------------------------------------------------------------------------

func TestParseCEFLEEF_CEF_Basic(t *testing.T) {
	msg := []byte("CEF:0|Security|threatmanager|1.0|100|worm successfully stopped|10|src=10.0.0.1 dst=2.1.2.2 spt=1232")
	header, ext, rawExt, ok := ParseCEFLEEF(msg)
	require.True(t, ok)

	assert.Equal(t, "CEF", header.Format)
	assert.Equal(t, "0", header.Version)
	assert.Equal(t, "Security", header.DeviceVendor)
	assert.Equal(t, "threatmanager", header.DeviceProduct)
	assert.Equal(t, "1.0", header.DeviceVersion)
	assert.Equal(t, "100", header.EventID)
	assert.Equal(t, "worm successfully stopped", header.Name)
	assert.Equal(t, "10", header.Severity)

	assert.Equal(t, "src=10.0.0.1 dst=2.1.2.2 spt=1232", string(rawExt))
	assert.Equal(t, "10.0.0.1", ext["src"])
	assert.Equal(t, "2.1.2.2", ext["dst"])
	assert.Equal(t, "1232", ext["spt"])
}

func TestParseCEFLEEF_CEF_Version1(t *testing.T) {
	msg := []byte("CEF:1|Security|threatmanager|1.0|100|worm successfully stopped|10|src=10.0.0.1")
	header, _, _, ok := ParseCEFLEEF(msg)
	require.True(t, ok)
	assert.Equal(t, "1", header.Version)
}

func TestParseCEFLEEF_CEF_EscapedPipeInHeader(t *testing.T) {
	msg := []byte(`CEF:0|Vendor\|Inc|Product|1.0|100|Name|5|src=1.2.3.4`)
	header, _, _, ok := ParseCEFLEEF(msg)
	require.True(t, ok)
	assert.Equal(t, "Vendor|Inc", header.DeviceVendor)
}

func TestParseCEFLEEF_CEF_EscapedBackslashInHeader(t *testing.T) {
	msg := []byte(`CEF:0|Vendor\\Corp|Product|1.0|100|Name|5|src=1.2.3.4`)
	header, _, _, ok := ParseCEFLEEF(msg)
	require.True(t, ok)
	assert.Equal(t, `Vendor\Corp`, header.DeviceVendor)
}

func TestParseCEFLEEF_CEF_EmptyExtension(t *testing.T) {
	msg := []byte("CEF:0|Vendor|Product|1.0|100|Name|5|")
	header, ext, rawExt, ok := ParseCEFLEEF(msg)
	require.True(t, ok)
	assert.Equal(t, "CEF", header.Format)
	assert.Equal(t, "", string(rawExt))
	assert.Nil(t, ext)
}

func TestParseCEFLEEF_CEF_NoExtension(t *testing.T) {
	// Missing final pipe means only 6 fields found, not 7.
	msg := []byte("CEF:0|Vendor|Product|1.0|100|Name|5")
	_, _, _, ok := ParseCEFLEEF(msg)
	assert.False(t, ok)
}

func TestParseCEFLEEF_CEF_TooFewPipes(t *testing.T) {
	msg := []byte("CEF:0|Vendor|Product|1.0|100")
	_, _, _, ok := ParseCEFLEEF(msg)
	assert.False(t, ok)
}

func TestParseCEFLEEF_CEF_EmptyMsg(t *testing.T) {
	msg := []byte("CEF:")
	_, _, _, ok := ParseCEFLEEF(msg)
	assert.False(t, ok)
}

// ---------------------------------------------------------------------------
// LEEF header parsing
// ---------------------------------------------------------------------------

func TestParseCEFLEEF_LEEF10_Basic(t *testing.T) {
	msg := []byte("LEEF:1.0|Microsoft|MSExchange|2013 SP1|15345|src=10.0.1.7\tdst=10.0.0.5\tsev=5")
	header, ext, rawExt, ok := ParseCEFLEEF(msg)
	require.True(t, ok)

	assert.Equal(t, "LEEF", header.Format)
	assert.Equal(t, "1.0", header.Version)
	assert.Equal(t, "Microsoft", header.DeviceVendor)
	assert.Equal(t, "MSExchange", header.DeviceProduct)
	assert.Equal(t, "2013 SP1", header.DeviceVersion)
	assert.Equal(t, "15345", header.EventID)
	assert.Empty(t, header.Name)
	assert.Empty(t, header.Severity)

	assert.Equal(t, "src=10.0.1.7\tdst=10.0.0.5\tsev=5", string(rawExt))
	assert.Equal(t, "10.0.1.7", ext["src"])
	assert.Equal(t, "10.0.0.5", ext["dst"])
	assert.Equal(t, "5", ext["sev"])
}

func TestParseCEFLEEF_LEEF20_TabDelimiter(t *testing.T) {
	msg := []byte("LEEF:2.0|NXLog|MyApp|5.5|91|0x09|key1=val1\tkey2=val2")
	header, ext, _, ok := ParseCEFLEEF(msg)
	require.True(t, ok)

	assert.Equal(t, "LEEF", header.Format)
	assert.Equal(t, "2.0", header.Version)
	assert.Equal(t, "NXLog", header.DeviceVendor)
	assert.Equal(t, "91", header.EventID)
	assert.Equal(t, "val1", ext["key1"])
	assert.Equal(t, "val2", ext["key2"])
}

func TestParseCEFLEEF_LEEF20_CaretDelimiter(t *testing.T) {
	msg := []byte("LEEF:2.0|Lancope|StealthWatch|1.0|41|^|src=10.0.1.8^dst=10.0.0.5^sev=5")
	header, ext, _, ok := ParseCEFLEEF(msg)
	require.True(t, ok)

	assert.Equal(t, "LEEF", header.Format)
	assert.Equal(t, "1.0", header.DeviceVersion)
	assert.Equal(t, "10.0.1.8", ext["src"])
	assert.Equal(t, "10.0.0.5", ext["dst"])
	assert.Equal(t, "5", ext["sev"])
	_ = header
}

func TestParseCEFLEEF_LEEF20_HexDelimiter(t *testing.T) {
	msg := []byte("LEEF:2.0|Vendor|Product|1.0|100|0x5E|key1=a^key2=b")
	_, ext, _, ok := ParseCEFLEEF(msg)
	require.True(t, ok)
	assert.Equal(t, "a", ext["key1"])
	assert.Equal(t, "b", ext["key2"])
}

func TestParseCEFLEEF_LEEF20_LowercaseHex(t *testing.T) {
	msg := []byte("LEEF:2.0|Vendor|Product|1.0|100|x7c|key1=a|key2=b")
	_, ext, _, ok := ParseCEFLEEF(msg)
	require.True(t, ok)
	assert.Equal(t, "a", ext["key1"])
	assert.Equal(t, "b", ext["key2"])
}

func TestParseCEFLEEF_LEEF_EmptyExtension(t *testing.T) {
	msg := []byte("LEEF:1.0|Vendor|Product|1.0|100|")
	header, ext, _, ok := ParseCEFLEEF(msg)
	require.True(t, ok)
	assert.Equal(t, "LEEF", header.Format)
	assert.Nil(t, ext)
}

func TestParseCEFLEEF_LEEF_TooFewPipes(t *testing.T) {
	msg := []byte("LEEF:1.0|Vendor|Product|1.0")
	_, _, _, ok := ParseCEFLEEF(msg)
	assert.False(t, ok)
}

func TestParseCEFLEEF_LEEF20_TooFewPipes(t *testing.T) {
	msg := []byte("LEEF:2.0|Vendor|Product|1.0|100")
	_, _, _, ok := ParseCEFLEEF(msg)
	assert.False(t, ok)
}

// ---------------------------------------------------------------------------
// Non-CEF/LEEF messages
// ---------------------------------------------------------------------------

func TestParseCEFLEEF_NotCEFOrLEEF(t *testing.T) {
	cases := [][]byte{
		[]byte("just a regular message"),
		[]byte(""),
		[]byte("CEF"),
		[]byte("LEEF"),
		[]byte("CEF 0"),
		[]byte("LEEF 1.0"),
		nil,
	}
	for _, msg := range cases {
		_, _, _, ok := ParseCEFLEEF(msg)
		assert.False(t, ok, "should not match: %q", msg)
	}
}

// ---------------------------------------------------------------------------
// CEF extension parsing
// ---------------------------------------------------------------------------

func TestParseCEFExtension_SimpleKV(t *testing.T) {
	ext := []byte("src=10.0.0.1 dst=2.1.2.2 spt=1232")
	result := parseCEFExtension(ext)
	assert.Equal(t, "10.0.0.1", result["src"])
	assert.Equal(t, "2.1.2.2", result["dst"])
	assert.Equal(t, "1232", result["spt"])
}

func TestParseCEFExtension_ValueWithSpaces(t *testing.T) {
	ext := []byte("filePath=/user/username/dir/my file name.txt dst=10.0.0.1")
	result := parseCEFExtension(ext)
	assert.Equal(t, "/user/username/dir/my file name.txt", result["filePath"])
	assert.Equal(t, "10.0.0.1", result["dst"])
}

func TestParseCEFExtension_EscapedEquals(t *testing.T) {
	ext := []byte(`src=10.0.0.1 act=blocked a \= dst=1.1.1.1`)
	result := parseCEFExtension(ext)
	assert.Equal(t, "10.0.0.1", result["src"])
	assert.Equal(t, "blocked a =", result["act"])
	assert.Equal(t, "1.1.1.1", result["dst"])
}

func TestParseCEFExtension_EscapedBackslash(t *testing.T) {
	ext := []byte(`src=10.0.0.1 act=blocked a \\ dst=1.1.1.1`)
	result := parseCEFExtension(ext)
	assert.Equal(t, `blocked a \`, result["act"])
	assert.Equal(t, "1.1.1.1", result["dst"])
}

func TestParseCEFExtension_EscapedNewline(t *testing.T) {
	ext := []byte(`msg=Detected a threat.\nNo action needed`)
	result := parseCEFExtension(ext)
	assert.Equal(t, "Detected a threat.\nNo action needed", result["msg"])
}

func TestParseCEFExtension_EscapedCR(t *testing.T) {
	ext := []byte(`msg=line1\rline2`)
	result := parseCEFExtension(ext)
	assert.Equal(t, "line1\rline2", result["msg"])
}

func TestParseCEFExtension_EmptyValue(t *testing.T) {
	ext := []byte("key= dst=10.0.0.1")
	result := parseCEFExtension(ext)
	assert.Equal(t, "", result["key"])
	assert.Equal(t, "10.0.0.1", result["dst"])
}

func TestParseCEFExtension_SinglePair(t *testing.T) {
	ext := []byte("src=10.0.0.1")
	result := parseCEFExtension(ext)
	assert.Equal(t, "10.0.0.1", result["src"])
	assert.Equal(t, 1, len(result))
}

func TestParseCEFExtension_DuplicateKeys(t *testing.T) {
	ext := []byte("src=first src=second")
	result := parseCEFExtension(ext)
	assert.Equal(t, "second", result["src"])
}

func TestParseCEFExtension_Empty(t *testing.T) {
	assert.Nil(t, parseCEFExtension([]byte("")))
	assert.Nil(t, parseCEFExtension(nil))
}

func TestParseCEFExtension_OnlyWhitespace(t *testing.T) {
	assert.Nil(t, parseCEFExtension([]byte("   ")))
}

func TestParseCEFExtension_KeyWithUnderscore(t *testing.T) {
	ext := []byte("custom_field=hello world dst=10.0.0.1")
	result := parseCEFExtension(ext)
	assert.Equal(t, "hello world", result["custom_field"])
	assert.Equal(t, "10.0.0.1", result["dst"])
}

func TestParseCEFExtension_PipeInValue(t *testing.T) {
	// Per spec, pipes in extension values do NOT need escaping.
	ext := []byte("act=blocked a | dst=1.1.1.1")
	result := parseCEFExtension(ext)
	assert.Equal(t, "blocked a |", result["act"])
	assert.Equal(t, "1.1.1.1", result["dst"])
}

func TestParseCEFExtension_TrailingSpacesLastValue(t *testing.T) {
	ext := []byte("src=10.0.0.1   ")
	result := parseCEFExtension(ext)
	assert.Equal(t, "10.0.0.1", result["src"])
}

// ---------------------------------------------------------------------------
// LEEF extension parsing
// ---------------------------------------------------------------------------

func TestParseLEEFExtension_TabDelimited(t *testing.T) {
	ext := []byte("src=10.0.0.1\tdst=10.0.0.2\tsev=5")
	result := parseLEEFExtension(ext, '\t')
	assert.Equal(t, "10.0.0.1", result["src"])
	assert.Equal(t, "10.0.0.2", result["dst"])
	assert.Equal(t, "5", result["sev"])
}

func TestParseLEEFExtension_CustomDelimiter(t *testing.T) {
	ext := []byte("src=10.0.0.1^dst=10.0.0.2^sev=5")
	result := parseLEEFExtension(ext, '^')
	assert.Equal(t, "10.0.0.1", result["src"])
	assert.Equal(t, "10.0.0.2", result["dst"])
	assert.Equal(t, "5", result["sev"])
}

func TestParseLEEFExtension_ValueWithEquals(t *testing.T) {
	ext := []byte("key1=a=b\tkey2=c")
	result := parseLEEFExtension(ext, '\t')
	assert.Equal(t, "a=b", result["key1"])
	assert.Equal(t, "c", result["key2"])
}

func TestParseLEEFExtension_EmptyValue(t *testing.T) {
	ext := []byte("key1=\tkey2=val")
	result := parseLEEFExtension(ext, '\t')
	assert.Equal(t, "", result["key1"])
	assert.Equal(t, "val", result["key2"])
}

func TestParseLEEFExtension_DuplicateKeys(t *testing.T) {
	ext := []byte("key=first\tkey=second")
	result := parseLEEFExtension(ext, '\t')
	assert.Equal(t, "second", result["key"])
}

func TestParseLEEFExtension_Empty(t *testing.T) {
	assert.Nil(t, parseLEEFExtension([]byte(""), '\t'))
	assert.Nil(t, parseLEEFExtension(nil, '\t'))
}

func TestParseLEEFExtension_NoEquals(t *testing.T) {
	// Malformed token without '=' is skipped.
	ext := []byte("noequals\tkey=val")
	result := parseLEEFExtension(ext, '\t')
	assert.Equal(t, "val", result["key"])
	assert.Equal(t, 1, len(result))
}

// ---------------------------------------------------------------------------
// parseLEEFDelimiter
// ---------------------------------------------------------------------------

func TestParseLEEFDelimiter(t *testing.T) {
	tests := []struct {
		input string
		want  byte
		ok    bool
	}{
		{"^", '^', true},
		{"|", '|', true},
		{"0x09", '\t', true},
		{"0x5E", '^', true},
		{"x7c", '|', true},
		{"0X09", '\t', true},
		{"X5E", '^', true},
		{"", 0, false},
		{"ab", 0, false},   // two literal chars, not hex
		{"0xZZ", 0, false}, // invalid hex
		{"0x", 0, false},   // hex prefix with no digits
	}
	for _, tc := range tests {
		got, ok := parseLEEFDelimiter(tc.input)
		assert.Equal(t, tc.ok, ok, "input=%q", tc.input)
		if ok {
			assert.Equal(t, tc.want, got, "input=%q", tc.input)
		}
	}
}

// ---------------------------------------------------------------------------
// unescapeCEFHeader
// ---------------------------------------------------------------------------

func TestUnescapeCEFHeader(t *testing.T) {
	assert.Equal(t, `Vendor|Inc`, unescapeCEFHeader(`Vendor\|Inc`))
	assert.Equal(t, `Vendor\Corp`, unescapeCEFHeader(`Vendor\\Corp`))
	assert.Equal(t, `No escapes`, unescapeCEFHeader(`No escapes`))
	assert.Equal(t, `Trailing\`, unescapeCEFHeader(`Trailing\`))
	assert.Equal(t, `A|B\C`, unescapeCEFHeader(`A\|B\\C`))
}

// ---------------------------------------------------------------------------
// unescapeCEFValue
// ---------------------------------------------------------------------------

func TestUnescapeCEFValue(t *testing.T) {
	assert.Equal(t, `hello=world`, unescapeCEFValue(`hello\=world`))
	assert.Equal(t, `back\slash`, unescapeCEFValue(`back\\slash`))
	assert.Equal(t, "line1\nline2", unescapeCEFValue(`line1\nline2`))
	assert.Equal(t, "line1\rline2", unescapeCEFValue(`line1\rline2`))
	assert.Equal(t, `no escapes`, unescapeCEFValue(`no escapes`))
	assert.Equal(t, `trailing\`, unescapeCEFValue(`trailing\`))
}

// ---------------------------------------------------------------------------
// BuildSIEMFields
// ---------------------------------------------------------------------------

func TestBuildSIEMFields_CEF(t *testing.T) {
	header := SIEMHeader{
		Format:        "CEF",
		Version:       "0",
		DeviceVendor:  "Security",
		DeviceProduct: "Firewall",
		DeviceVersion: "1.0",
		EventID:       "100",
		Name:          "Attack",
		Severity:      "10",
	}
	ext := map[string]string{"src": "1.2.3.4"}
	fields := BuildSIEMFields(header, ext)

	assert.Equal(t, "CEF", fields["format"])
	assert.Equal(t, "0", fields["version"])
	assert.Equal(t, "Security", fields["device_vendor"])
	assert.Equal(t, "Firewall", fields["device_product"])
	assert.Equal(t, "1.0", fields["device_version"])
	assert.Equal(t, "100", fields["event_id"])
	assert.Equal(t, "Attack", fields["name"])
	assert.Equal(t, "10", fields["severity"])
	assert.Equal(t, map[string]string{"src": "1.2.3.4"}, fields["extension"])
}

func TestBuildSIEMFields_LEEF(t *testing.T) {
	header := SIEMHeader{
		Format:        "LEEF",
		Version:       "1.0",
		DeviceVendor:  "Microsoft",
		DeviceProduct: "MSExchange",
		DeviceVersion: "2013 SP1",
		EventID:       "15345",
	}
	fields := BuildSIEMFields(header, nil)

	assert.Equal(t, "LEEF", fields["format"])
	_, hasName := fields["name"]
	assert.False(t, hasName)
	_, hasSev := fields["severity"]
	assert.False(t, hasSev)
	_, hasExt := fields["extension"]
	assert.False(t, hasExt)
}
