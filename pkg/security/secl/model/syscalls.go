// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package model holds model related files
package model

import (
	"strings"
	"unicode"
)

// MarshalText maps the syscall identifier to UTF-8-encoded text and returns the result
func (s Syscall) MarshalText() ([]byte, error) {
	return []byte(strings.ToLower(strings.TrimPrefix(s.String(), "Sys"))), nil
}

// ConvertSyscallName converts syscall into a unix format
func (s Syscall) ConvertSyscallName() string {
	var result []rune
	for i, r := range s.String()[3:] {
		if unicode.IsUpper(r) && i > 0 {
			result = append(result, '_')
		}
		result = append(result, unicode.ToLower(r))
	}
	return string(result)
}
