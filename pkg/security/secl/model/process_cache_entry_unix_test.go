// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

// Package model holds model related files
package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasValidLineage(t *testing.T) {
	newPCE := func(pid uint32, parent *ProcessCacheEntry, isParentMissing bool) *ProcessCacheEntry {
		pce := &ProcessCacheEntry{
			ProcessContext: ProcessContext{
				Process: Process{
					PIDContext: PIDContext{
						Pid: pid,
					},

					IsParentMissing: isParentMissing,
				},
				Ancestor: parent,
			},
		}
		if parent != nil {
			pce.PPid = parent.Pid
		}

		return pce
	}

	t.Run("valid", func(t *testing.T) {
		pid1 := newPCE(1, nil, false)
		child1 := newPCE(2, pid1, false)
		child2 := newPCE(3, child1, false)

		isValid, err := child2.HasValidLineage()
		assert.True(t, isValid)
		assert.Nil(t, err)
	})

	t.Run("pid1-missing", func(t *testing.T) {
		child1 := newPCE(2, nil, false)
		child2 := newPCE(3, child1, false)

		isValid, err := child2.HasValidLineage()
		assert.False(t, isValid)
		assert.NotNil(t, err)

		var mn *ErrProcessIncompleteLineage
		assert.ErrorAs(t, err, &mn)
	})

	t.Run("two-pid1", func(t *testing.T) {
		pid1 := newPCE(1, nil, false)
		child1 := newPCE(1, pid1, false)
		child2 := newPCE(3, child1, false)

		isValid, err := child2.HasValidLineage()
		assert.False(t, isValid)
		assert.NotNil(t, err)

		var mn *ErrProcessWrongParentNode
		assert.ErrorAs(t, err, &mn)
	})

	t.Run("parent-missing", func(t *testing.T) {
		pid1 := newPCE(1, nil, false)
		child1 := newPCE(2, pid1, true)
		child2 := newPCE(3, child1, false)

		isValid, err := child2.HasValidLineage()
		assert.False(t, isValid)
		assert.NotNil(t, err)

		var mn *ErrProcessMissingParentNode
		assert.ErrorAs(t, err, &mn)
	})
}
