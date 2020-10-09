// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package traceutil

// TruncateUTF8 truncates the given string to make sure it uses less than limit bytes.
// If the last character is an utf8 character that would be splitten, it removes it
// entirely to make sure the resulting string is not broken.
func TruncateUTF8(s string, limit int) string {
	var lastValidIndex int
	for i := range s {
		if i > limit {
			return s[:lastValidIndex]
		}
		lastValidIndex = i
	}
	return s
}
