// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package otelthreadctx

import "strconv"

// Attribute is one attrs_data entry, see NamedAttributes for its key name.
type Attribute struct {
	KeyIndex uint8
	Value    string
}

// ParseAttributes decodes repeated [key index u8][value len u8][value]. A
// trailing entry cut by the read clamp is dropped.
func ParseAttributes(attrsData []byte) []Attribute {
	var attrs []Attribute

	for off := 0; off+2 <= len(attrsData); {
		keyIndex := attrsData[off]
		valLen := int(attrsData[off+1])
		off += 2

		if off+valLen > len(attrsData) {
			break
		}
		attrs = append(attrs, Attribute{KeyIndex: keyIndex, Value: string(attrsData[off : off+valLen])})
		off += valLen
	}

	return attrs
}

// NamedAttributes maps attrs by the key names the process context lists.
func NamedAttributes(keyNames []string, attrs []Attribute) map[string]string {
	if len(attrs) == 0 {
		return nil
	}

	named := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		named[attributeName(keyNames, attr.KeyIndex)] = attr.Value
	}
	return named
}

func attributeName(keyNames []string, keyIndex uint8) string {
	if int(keyIndex) < len(keyNames) {
		return keyNames[keyIndex]
	}
	return strconv.Itoa(int(keyIndex))
}
