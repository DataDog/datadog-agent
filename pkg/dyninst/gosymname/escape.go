// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gosymname

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// splitPkg splits a full linker symbol name into package (full import path,
// still escaped) and local symbol name.
//
// Adapted from
// https://github.com/golang/go/blob/c0025d5e0b3f6fca7117e9b8f4593a95e37a9fa5/src/cmd/compile/internal/ir/func.go#L367
func splitPkg(name string) (pkgpath, sym string) {
	// Package-sym split is at first dot after the last '/' that comes before
	// any characters illegal in an escaped package path.
	lastSlashIdx := 0
	for i, r := range name {
		if !escapedImportPathOK(r) {
			break
		}
		if r == '/' {
			lastSlashIdx = i
		}
	}
	for i := lastSlashIdx; i < len(name); i++ {
		if name[i] == '.' {
			return name[:i], name[i+1:]
		}
	}
	return "", name
}

// unescapePkg unescapes a package import path, replacing %XX escape sequences
// with the original characters. Returns the unescaped path.
func unescapePkg(s string) (string, error) {
	if !strings.Contains(s, "%") {
		return s, nil
	}

	p := make([]byte, 0, len(s))
	for i := 0; i < len(s); {
		if s[i] != '%' {
			p = append(p, s[i])
			i++
			continue
		}
		if i+2 >= len(s) {
			return "", fmt.Errorf("malformed prefix %q: escape sequence must contain two hex digits", s)
		}
		b, err := strconv.ParseUint(s[i+1:i+3], 16, 8)
		if err != nil {
			return "", fmt.Errorf("malformed prefix %q: escape sequence %q must contain two hex digits", s, s[i:i+3])
		}
		p = append(p, byte(b))
		i += 3
	}
	return string(p), nil
}

func modPathOK(r rune) bool {
	if r < utf8.RuneSelf {
		return r == '-' || r == '.' || r == '_' || r == '~' ||
			'0' <= r && r <= '9' ||
			'A' <= r && r <= 'Z' ||
			'a' <= r && r <= 'z'
	}
	return false
}

func escapedImportPathOK(r rune) bool {
	return modPathOK(r) || r == '+' || r == '/' || r == '%'
}
