// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package ntfsdu

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/ntfsmft"
	"github.com/stretchr/testify/assert"
)

func TestEffectiveTreeDepth(t *testing.T) {
	for _, tc := range []struct {
		name      string
		maxDepth  int
		treeLimit int
		wantDepth int
	}{
		{name: "depth zero", maxDepth: 0, treeLimit: 100, wantDepth: 0},
		{name: "tree limit zero", maxDepth: 16, treeLimit: 0, wantDepth: 0},
		{name: "tree enabled", maxDepth: 2, treeLimit: 1, wantDepth: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := effectiveTreeDepth(tc.maxDepth, tc.treeLimit); got != tc.wantDepth {
				t.Errorf("effectiveTreeDepth(%d, %d) = %d, want %d", tc.maxDepth, tc.treeLimit, got, tc.wantDepth)
			}
		})
	}
}

func TestFlattenTreeLimitKeepsExistingPreorder(t *testing.T) {
	root := &ntfsmft.TreeNode{
		Name: "root",
		Children: []*ntfsmft.TreeNode{
			{Name: "largest", Depth: 1, Size: 30, Children: []*ntfsmft.TreeNode{{Name: "nested", Depth: 2, Size: 20}}},
			{Name: "smaller", Depth: 1, Size: 10},
		},
	}

	nodes := flattenTree(root, 2, 3)
	assert.Len(t, nodes, 3)
	assert.Equal(t, []string{"root", "root\\largest", "root\\largest\\nested"}, []string{nodes[0].Path, nodes[1].Path, nodes[2].Path})
	assert.Empty(t, flattenTree(root, 2, 0))
}
