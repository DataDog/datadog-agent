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

func newPCE(pid uint32, parent *ProcessCacheEntry, isParentMissing bool) *ProcessCacheEntry {
	pce := &ProcessCacheEntry{}
	pce.Pid = pid
	pce.IsParentMissing = isParentMissing
	if parent != nil {
		pce.PPid = parent.Pid
		pce.setAncestor(parent)
	}
	return pce
}

func TestHasValidLineage(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		pid1 := newPCE(1, nil, false)
		child1 := newPCE(2, pid1, false)
		child2 := newPCE(3, child1, false)

		isValid, err := child2.HasValidLineage()
		assert.True(t, isValid)
		assert.NoError(t, err)
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

}

func TestEntryEquals(t *testing.T) {
	e1 := NewProcessCacheEntry()
	e1.Pid = 2
	e2 := NewProcessCacheEntry()
	e2.Pid = 3
	assert.True(t, e1.Equals(e2))

	// different file
	e1.FileEvent.Inode = 33
	e2.FileEvent.Inode = 44
	assert.False(t, e1.Equals(e2))

	// same file
	e2.FileEvent.Inode = 33
	assert.True(t, e1.Equals(e2))

	// different args
	e2.ArgsEntry = &ArgsEntry{Values: []string{"aaa"}}
	assert.False(t, e1.Equals(e2))

	// same args
	e1.ArgsEntry = &ArgsEntry{Values: []string{"aaa"}}
	assert.True(t, e1.Equals(e2))
}

func TestCopyProcessContextFromParent(t *testing.T) {
	parent := newPCE(1, nil, false)

	parent.CGroup = CGroupContext{
		CGroupPathKey: PathKey{
			MountID: 1234,
			Inode:   5678,
		},
		CGroupID: "1234",
	}
	parent.ContainerContext = ContainerContext{
		ContainerID: "1234",
	}
	parent.MntNS = 1234
	parent.NetNS = 5678
	parent.UserSession = UserSessionContext{
		SSHSessionContext: SSHSessionContext{
			SSHSessionID: 9876,
		},
	}
	parent.Credentials = Credentials{
		AUID: 1234,
	}

	t.Run("fork", func(t *testing.T) {
		child := newPCE(2, nil, false)
		parent.Fork(child)

		assert.Equal(t, parent.CGroup, child.CGroup)
		assert.Equal(t, parent.ContainerContext, child.ContainerContext)
		assert.Equal(t, parent.MntNS, child.MntNS)
		assert.Equal(t, parent.NetNS, child.NetNS)
		assert.Equal(t, parent.UserSession.SSHSessionContext, child.UserSession.SSHSessionContext)
		assert.Equal(t, parent.Credentials, child.Credentials)
	})

	t.Run("exec", func(t *testing.T) {
		child := newPCE(2, nil, false)
		parent.Exec(child)

		assert.Equal(t, parent.CGroup, child.CGroup)
		assert.Equal(t, parent.ContainerContext, child.ContainerContext)
		assert.Equal(t, parent.MntNS, child.MntNS)
		assert.Equal(t, parent.NetNS, child.NetNS)
		assert.Equal(t, parent.UserSession.SSHSessionContext, child.UserSession.SSHSessionContext)
		assert.Equal(t, parent.Credentials, child.Credentials)
	})

	t.Run("set-fork-parent", func(t *testing.T) {
		child := newPCE(2, nil, false)
		child.SetForkParent(parent)

		assert.Equal(t, parent.CGroup, child.CGroup)
		assert.Equal(t, parent.ContainerContext, child.ContainerContext)
		assert.Equal(t, parent.MntNS, child.MntNS)
		assert.Equal(t, parent.NetNS, child.NetNS)
		assert.Equal(t, parent.UserSession.SSHSessionContext, child.UserSession.SSHSessionContext)
		assert.Equal(t, parent.Credentials, child.Credentials)
	})

	t.Run("set-exec-parent", func(t *testing.T) {
		child := newPCE(2, nil, false)
		child.SetExecParent(parent)

		assert.Equal(t, parent.CGroup, child.CGroup)
		assert.Equal(t, parent.ContainerContext, child.ContainerContext)
		assert.Equal(t, parent.MntNS, child.MntNS)
		assert.Equal(t, parent.NetNS, child.NetNS)
		assert.Equal(t, parent.UserSession.SSHSessionContext, child.UserSession.SSHSessionContext)
		assert.Equal(t, parent.Credentials, child.Credentials)
	})
}
