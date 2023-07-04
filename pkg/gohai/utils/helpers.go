// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package utils

import "bytes"

// StringFromBytes converts a null-terminated (C-style) string to a Go string
//
// The given slice must be null-terminated.
func StringFromBytes(slice []byte) string {
	// using `string(slice)` will keep the null bytes in the resulting Go string, so we have to
	// check for the position of the first null byte and troncate the slice
	length := bytes.IndexByte(slice, 0)
	if length == -1 {
		length = len(slice)
	}
	return string(slice[:length])
}
