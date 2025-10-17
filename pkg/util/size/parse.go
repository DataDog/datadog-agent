// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package size

import (
	"strings"
	"unicode"

	"github.com/spf13/cast"
)

// ParseSizeInBytes converts strings like 1GB or 12 mb into an unsigned integer number of bytes.
// The function is case-insensitive and handles the following units:
// - b or B for bytes
// - kb or KB for kilobytes (1024 bytes)
// - mb or MB for megabytes (1024^2 bytes)
// - gb or GB for gigabytes (1024^3 bytes)
func ParseSizeInBytes(sizeStr string) uint {
	sizeStr = strings.TrimSpace(sizeStr)
	lastChar := len(sizeStr) - 1
	multiplier := uint(1)

	if lastChar > 0 {
		if sizeStr[lastChar] == 'b' || sizeStr[lastChar] == 'B' {
			if lastChar > 1 {
				switch unicode.ToLower(rune(sizeStr[lastChar-1])) {
				case 'k':
					multiplier = 1 << 10
					sizeStr = strings.TrimSpace(sizeStr[:lastChar-1])
				case 'm':
					multiplier = 1 << 20
					sizeStr = strings.TrimSpace(sizeStr[:lastChar-1])
				case 'g':
					multiplier = 1 << 30
					sizeStr = strings.TrimSpace(sizeStr[:lastChar-1])
				default:
					// Handle case where it's just "1B" or similar
					sizeStr = strings.TrimSpace(sizeStr[:lastChar])
				}
			} else {
				// Handle case where it's just "B" (1 byte)
				sizeStr = "1"
			}
		}
	}

	size := max(cast.ToInt(sizeStr), 0)
	return safeMul(uint(size), multiplier)
}

// safeMul performs a safe multiplication that checks for overflow
func safeMul(a, b uint) uint {
	c := a * b
	// detect multiplication overflows
	if a > 1 && b > 1 && c/b != a {
		return 0
	}
	return c
}
