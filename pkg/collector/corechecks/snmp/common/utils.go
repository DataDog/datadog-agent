// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"bytes"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/version"
)

// CreateStringBatches batches strings into chunks with specific size
func CreateStringBatches(elements []string, size int) ([][]string, error) {
	var batches [][]string

	if size <= 0 {
		return nil, fmt.Errorf("batch size must be positive. invalid size: %d", size)
	}

	for i := 0; i < len(elements); i += size {
		j := i + size
		if j > len(elements) {
			j = len(elements)
		}
		batch := elements[i:j]
		batches = append(batches, batch)
	}

	return batches, nil
}

// CopyStrings makes a copy of a list of strings
func CopyStrings(tags []string) []string {
	newTags := make([]string, len(tags))
	copy(newTags, tags)
	return newTags
}

// GetAgentVersionTag returns agent version tag
func GetAgentVersionTag() string {
	return "agent_version:" + version.AgentVersion
}

// NormalizeHost applies a liberal policy on host names.
func NormalizeHost(host string) string {
	var buf bytes.Buffer

	// hosts longer than 253 characters are illegal
	if len(host) > 253 {
		return ""
	}

	for _, r := range host {
		switch r {
		// has null rune just toss the whole thing
		case '\x00':
			return ""
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

	return buf.String()
}
