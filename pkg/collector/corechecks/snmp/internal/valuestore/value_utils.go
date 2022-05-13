// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package valuestore

var strippableSpecialChars = map[byte]bool{'\r': true, '\n': true, '\t': true}

func isString(bytesValue []byte) bool {
	for _, bit := range bytesValue {
		if bit < 32 || bit > 126 {
			// The char is not a printable ASCII char but it might be a character that
			// can be stripped like `\n`
			if _, ok := strippableSpecialChars[bit]; !ok {
				return false
			}
		}
	}
	return true
}
