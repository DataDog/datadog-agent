// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package common

import (
	"fmt"
	"golang.org/x/text/encoding/unicode"
)

// ConvertUTF16ToUTF8 converts a byte slice from UTF-16 to UTF-8
//
// UTF-16 little-endian (UTF-16LE) is the encoding standard in the Windows operating system.
// https://learn.microsoft.com/en-us/globalization/encoding/transformations-of-unicode-code-points
func ConvertUTF16ToUTF8(content []byte) ([]byte, error) {
	utf16 := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	utf8, err := utf16.NewDecoder().Bytes(content)
	if err != nil {
		return nil, fmt.Errorf("failed to convert UTF-16 to UTF-8: %v", err)
	}
	return utf8, nil
}
