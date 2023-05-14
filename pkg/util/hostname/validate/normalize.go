// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package validate

import (
	"bytes"
	"fmt"
	"regexp"
)

var (
	// Filter to clean the directory name from invalid file name characters
	directoryNameFilter = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)
)

const (
	// Maximum size for a directory name
	directoryToHostnameMaxSize = 32
)

// NormalizeHost applies a liberal policy on host names.
func NormalizeHost(host string) (string, error) {
	var buf bytes.Buffer

	// hosts longer than 253 characters are illegal
	if len(host) > 253 {
		return "", fmt.Errorf("hostname is too long, should contain less than 253 characters")
	}

	for _, r := range host {
		switch r {
		// has null rune just toss the whole thing
		case '\x00':
			return "", fmt.Errorf("hostname cannot contain null character")
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

	return buf.String(), nil
}

// CleanHostnameDir returns a hostname normalized to be uses as a directory name.
func CleanHostnameDir(hostname string) string {
	hostname = directoryNameFilter.ReplaceAllString(hostname, "_")
	if len(hostname) > directoryToHostnameMaxSize {
		return hostname[:directoryToHostnameMaxSize]
	}
	return hostname
}
