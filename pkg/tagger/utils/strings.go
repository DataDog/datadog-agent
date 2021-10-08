// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import "strings"

// StringInRuneset tests whether all runes of a string are in a given subset
// returns false if any rune in the string is not found in the subset
func StringInRuneset(name, subset string) bool {
	for _, r := range name {
		if !strings.ContainsRune(subset, r) {
			// Found an unexpected rune in suffix
			return false
		}
	}
	return true
}
