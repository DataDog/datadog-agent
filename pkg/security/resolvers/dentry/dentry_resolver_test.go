// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package dentry holds dentry related files
package dentry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeFilenameFromParts(t *testing.T) {
	type testEntry struct {
		name     string
		parts    [][]byte
		expected string
	}

	entries := []testEntry{
		{
			name:     "empty",
			parts:    [][]byte{},
			expected: "/",
		},
		{
			name: "basic",
			parts: [][]byte{
				[]byte("a"),
				[]byte("b"),
				[]byte("c"),
			},
			expected: "/c/b/a",
		},
	}

	for _, entry := range entries {
		t.Run(entry.name, func(t *testing.T) {
			assert.Equal(t, entry.expected, computeFilenameFromParts(entry.parts))
		})
	}
}
