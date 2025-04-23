// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package normalize

import "unicode/utf8"

// TruncateUTF8 truncates the given string to make sure it uses less than limit bytes.
// If the last character is an utf8 character that would be splitten, it removes it
// entirely to make sure the resulting string is not broken.
func TruncateUTF8(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	s = s[:limit]
	// The max length of a valid code point is 4 bytes, therefore if we see all valid
	// code points in the last 4 bytes we know we have a fully valid utf-8 string
	// If not we can truncate one byte at a time until the end of the string is valid utf-8
	for len(s) >= 1 {
		if len(s) >= 4 && utf8.Valid([]byte(s[len(s)-4:])) {
			break
		}
		if len(s) >= 3 && utf8.Valid([]byte(s[len(s)-3:])) {
			break
		}
		if len(s) >= 2 && utf8.Valid([]byte(s[len(s)-2:])) {
			break
		}
		if len(s) >= 1 && utf8.Valid([]byte(s[len(s)-1:])) {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}
