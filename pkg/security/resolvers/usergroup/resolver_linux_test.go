// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package usergroup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

type stubCGroupFS struct{}

func (stubCGroupFS) FindCGroupContext(uint32, uint32) (containerutils.ContainerID, utils.CGroupContext, string, error) {
	return "", utils.CGroupContext{}, "", os.ErrNotExist
}

func (stubCGroupFS) GetCGroupPids(string) ([]uint32, error) {
	return nil, nil
}

func TestResolveUserMultiCGroupContainer(t *testing.T) {
	procRoot := t.TempDir()
	t.Setenv("HOST_PROC", procRoot)

	const livePID = uint32(2020)
	const stalePID = uint32(3030)
	etcDir := filepath.Join(procRoot, "2020", "root", "etc")
	require.NoError(t, os.MkdirAll(etcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(etcDir, "passwd"), []byte("appuser:x:1001:1001::/home/appuser:/bin/sh\n"), 0o644))

	cgroupResolver, err := cgroup.NewResolver(nil, stubCGroupFS{}, nil)
	require.NoError(t, err)

	userGroupResolver, err := NewResolver(cgroupResolver)
	require.NoError(t, err)

	const cid = containerutils.ContainerID("c40dff48f1d53c3f07a50aa12bb9ae0e58c0927dc6b1d77e3f166784722642ad")
	rootCGroup := model.CGroupContext{
		CGroupID:      containerutils.CGroupID("/kubepods.slice/cri-containerd-" + string(cid) + ".scope"),
		CGroupPathKey: model.PathKey{MountID: 42, Inode: 100},
		CGroupSource:  model.CGroupSourceEvent,
	}
	child1CGroup := model.CGroupContext{
		CGroupID:      containerutils.CGroupID("/kubepods.slice/cri-containerd-" + string(cid) + ".scope/init.scope"),
		CGroupPathKey: model.PathKey{MountID: 42, Inode: 200},
		CGroupSource:  model.CGroupSourceEvent,
	}
	child2CGroup := model.CGroupContext{
		CGroupID:      containerutils.CGroupID("/kubepods.slice/cri-containerd-" + string(cid) + ".scope/app.scope"),
		CGroupPathKey: model.PathKey{MountID: 42, Inode: 300},
		CGroupSource:  model.CGroupSourceEvent,
	}

	require.NotNil(t, cgroupResolver.Add(rootCGroup))
	require.NotNil(t, cgroupResolver.AddPID(livePID, child1CGroup))
	require.NotNil(t, cgroupResolver.AddPID(stalePID, child2CGroup))

	user, err := userGroupResolver.ResolveUser(1001, cid)
	require.NoError(t, err)
	assert.Equal(t, "appuser", user)

	userGroupResolver.nsUserCache.Purge()
	cgroupResolver.DelPID(stalePID)

	user, err = userGroupResolver.ResolveUser(1001, cid)
	require.NoError(t, err)
	assert.Equal(t, "appuser", user)
}
