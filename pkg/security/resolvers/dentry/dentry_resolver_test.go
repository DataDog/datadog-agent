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

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestDentryResolutionErrorsFormat(t *testing.T) {
	leaf := model.PathKey{MountID: 42, Inode: 1001, PathID: 7}
	parent := model.PathKey{MountID: 42, Inode: 2, PathID: 3}

	assert.Equal(t,
		"dentry path key not found 42/1001/7 (depth=0, caller=file)",
		(&ErrDentryPathKeyNotFound{LeafKey: leaf, PathKey: leaf, Depth: 0, Caller: CallerFile}).Error(),
	)
	assert.Equal(t,
		"dentry parent path key not found 42/2/3 at depth=2 (leaf=42/1001/7, caller=file)",
		(&ErrDentryPathKeyNotFound{LeafKey: leaf, PathKey: parent, Depth: 2, Caller: CallerFile}).Error(),
	)
	assert.Equal(t,
		"erpc_not_processed (leaf=42/1001/7, caller=file)",
		(&ErrERPCRequestNotProcessed{LeafKey: leaf, PathKey: leaf, Depth: 1, Caller: CallerFile}).Error(),
	)
	assert.Equal(t,
		"erpc_not_processed at depth=3 key=42/2/3 (leaf=42/1001/7, caller=file)",
		(&ErrERPCRequestNotProcessed{LeafKey: leaf, PathKey: parent, Depth: 3, Caller: CallerFile}).Error(),
	)
	assert.Equal(t,
		"erpc_resolution at depth=2 key=42/2/3 (leaf=42/1001/7, caller=file)",
		(&ErrERPCResolution{LeafKey: leaf, PathKey: parent, Depth: 2, Caller: CallerFile}).Error(),
	)
	assert.Equal(t,
		"map_resolution at depth=2 key=42/2/3 (leaf=42/1001/7, caller=file)",
		(&ErrKernelMapResolution{LeafKey: leaf, PathKey: parent, Depth: 2, Caller: CallerFile}).Error(),
	)
	assert.Equal(t,
		"truncated_parents_erpc at depth=125 key=42/2/3 (leaf=42/1001/7, caller=file)",
		(&ErrTruncatedParentsERPC{LeafKey: leaf, PathKey: parent, Depth: 125, Caller: CallerFile}).Error(),
	)
	assert.Equal(t,
		"truncated_parents at depth=125 key=42/2/3 (leaf=42/1001/7, caller=file)",
		(&ErrTruncatedParents{LeafKey: leaf, PathKey: parent, Depth: 125, Caller: CallerFile}).Error(),
	)
}

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
