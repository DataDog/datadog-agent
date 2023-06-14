// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package utils

import "bytes"

// StringFromBytes converts a null-terminated (C-style) string to a Go string
//
// The given slice must be null-terminated.
func StringFromBytes(slice []byte) string {
	length := bytes.IndexByte(slice, 0)
	return string(slice[:length])
}
