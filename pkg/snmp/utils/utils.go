// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//nolint:revive // TODO(NDM) Fix revive linter
package utils

import (
	"bytes"
	"fmt"
)

// NormalizeNamespace applies policy according to hostname rule
func NormalizeNamespace(namespace string) (string, error) {
	var buf bytes.Buffer

	// namespace longer than 100 characters are illegal
	if len(namespace) > 100 {
		return "", fmt.Errorf("namespace is too long, should contain less than 100 characters")
	}

	for _, r := range namespace {
		switch r {
		// has null rune just toss the whole thing
		case '\x00':
			return "", fmt.Errorf("namespace cannot contain null character")
		// drop these characters entirely
		case '\n', '\r', '\t':
			continue
		// replace characters that are generally used for xss with '-'
		case '>', '<':
			buf.WriteByte('-')
		default:
			buf.WriteRune(r)
		}
	}

	normalizedNamespace := buf.String()
	if normalizedNamespace == "" {
		return "", fmt.Errorf("namespace cannot be empty")
	}

	return normalizedNamespace, nil
}
