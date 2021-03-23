// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package model

import "unicode"

// IsPrintable returns whether the string does contain only ascii
func IsPrintable(s string) bool {
	for _, c := range s {
		if !unicode.IsOneOf(unicode.PrintRanges, c) {
			return false
		}
	}
	return true
}
