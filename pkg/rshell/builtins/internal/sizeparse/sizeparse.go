// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package sizeparse parses coreutils-style non-negative byte sizes.
package sizeparse

import (
	"errors"
	"strconv"
)

// ErrInvalid is returned for non-numeric, malformed, or out-of-range sizes.
var ErrInvalid = errors.New("invalid size")

// ErrRelative is returned when a size uses a relative-size operator.
var ErrRelative = errors.New("relative size operators not supported")

const maxInt64 = int64(1<<63 - 1)

var multipliers = map[string]int64{
	"":    1,
	"K":   1 << 10,
	"k":   1 << 10,
	"KiB": 1 << 10,
	"kiB": 1 << 10,
	"KB":  1000,
	"kB":  1000,
	"M":   1 << 20,
	"m":   1 << 20,
	"MiB": 1 << 20,
	"miB": 1 << 20,
	"MB":  1000 * 1000,
	"mB":  1000 * 1000,
	"G":   1 << 30,
	"g":   1 << 30,
	"GiB": 1 << 30,
	"giB": 1 << 30,
	"GB":  1000 * 1000 * 1000,
	"gB":  1000 * 1000 * 1000,
	"T":   1 << 40,
	"t":   1 << 40,
	"TiB": 1 << 40,
	"tiB": 1 << 40,
	"TB":  1000 * 1000 * 1000 * 1000,
	"tB":  1000 * 1000 * 1000 * 1000,
	"P":   1 << 50,
	"PiB": 1 << 50,
	"PB":  1000 * 1000 * 1000 * 1000 * 1000,
	"E":   1 << 60,
	"EiB": 1 << 60,
	"EB":  1000 * 1000 * 1000 * 1000 * 1000 * 1000,
}

// Parse parses a non-negative byte count with an optional coreutils suffix.
//
// For K/M/G/T the leading letter is case-insensitive; P/E are uppercase-only,
// matching GNU coreutils on 64-bit systems. Relative-size modifiers are
// rejected with ErrRelative so callers can surface a clearer message.
func Parse(s string) (int64, error) {
	if s == "" {
		return 0, ErrInvalid
	}
	switch s[0] {
	case '+', '-', '<', '>', '/', '%':
		return 0, ErrRelative
	}

	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, ErrInvalid
	}
	digits, suffix := s[:i], s[i:]

	mult, ok := multipliers[suffix]
	if !ok {
		return 0, ErrInvalid
	}

	n, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, ErrInvalid
	}
	if mult == 1 {
		return n, nil
	}
	if n > maxInt64/mult {
		return 0, ErrInvalid
	}
	return n * mult, nil
}
