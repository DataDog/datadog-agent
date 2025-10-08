// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroup

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// MockCGroupFS implements a mock for CGroupFSInterface
type MockCGroupFS struct {
	mock.Mock
}

func (m *MockCGroupFS) FindCGroupContext(tgid, pid uint32) (containerutils.ContainerID, utils.CGroupContext, string, error) {
	args := m.Called(tgid, pid)
	return args.Get(0).(containerutils.ContainerID), args.Get(1).(utils.CGroupContext), args.String(2), args.Error(3)
}

func (m *MockCGroupFS) GetCgroupPids(cgroupID string) ([]uint32, error) {
	args := m.Called(cgroupID)
	return args.Get(0).([]uint32), args.Error(1)
}

// createTestResolver creates a resolver with mocked dependencies for testing
func createTestResolver(t *testing.T) (*Resolver, *MockCGroupFS) {
	mockCGroupFS := &MockCGroupFS{}

	resolver, err := NewResolver(nil, mockCGroupFS)
	assert.NoError(t, err)

	return resolver, mockCGroupFS
}

// createTestProcess creates a ProcessCacheEntry for testing
func createTestProcess(pid, ppid uint32, containerID containerutils.ContainerID) *model.ProcessCacheEntry {
	process := &model.ProcessCacheEntry{
		ProcessContext: model.ProcessContext{
			Process: model.Process{
				PIDContext: model.PIDContext{
					Pid: pid,
				},
				PPid:        ppid,
				ContainerID: containerID,
				CGroup: model.CGroupContext{
					CGroupID: "",
					CGroupFile: model.PathKey{
						Inode:   0,
						MountID: 0,
					},
				},
			},
		},
	}
	return process
}

func TestResolvePidCgroupFallback_SuccessDirectResolution(t *testing.T) {
	resolver, mockFS := createTestResolver(t)
	process := createTestProcess(1234, 5678, "")

	expectedContext := utils.CGroupContext{
		CGroupID:          "test-cgroup-id",
		CGroupFileMountID: 42,
		CGroupFileInode:   9876,
	}

	// Mock successful direct resolution
	mockFS.On("FindCGroupContext", uint32(1234), uint32(1234)).Return(
		containerutils.ContainerID("container-123"),
		expectedContext,
		"/sys/fs/cgroup/test",
		nil,
	)

	result := resolver.resolvePidCgroupFallback(process)

	assert.True(t, result)
	assert.Equal(t, containerutils.CGroupID("test-cgroup-id"), process.CGroup.CGroupID)
	assert.Equal(t, uint32(42), process.CGroup.CGroupFile.MountID)
	assert.Equal(t, uint64(9876), process.CGroup.CGroupFile.Inode)
	assert.Equal(t, containerutils.ContainerID("container-123"), process.ContainerID)

	mockFS.AssertExpectations(t)
}

func TestResolvePidCgroupFallback_FailInvalidPPid(t *testing.T) {
	resolver, mockFS := createTestResolver(t)

	// Test case 1: PPid equals Pid
	process1 := createTestProcess(1234, 1234, "")
	mockFS.On("FindCGroupContext", uint32(1234), uint32(1234)).Return(
		containerutils.ContainerID(""),
		utils.CGroupContext{},
		"",
		errors.New("not found"),
	)

	result1 := resolver.resolvePidCgroupFallback(process1)
	assert.False(t, result1)

	// Test case 2: PPid is 0
	process2 := createTestProcess(1234, 0, "")
	mockFS.On("FindCGroupContext", uint32(1234), uint32(1234)).Return(
		containerutils.ContainerID(""),
		utils.CGroupContext{},
		"",
		errors.New("not found"),
	)

	result2 := resolver.resolvePidCgroupFallback(process2)
	assert.False(t, result2)

	mockFS.AssertExpectations(t)
}

func TestResolvePidCgroupFallback_SuccessFromHistory(t *testing.T) {
	resolver, mockFS := createTestResolver(t)
	process := createTestProcess(1234, 5678, "")

	// Add parent cgroup to history
	parentInode := uint64(9999)
	resolver.history.Add(uint32(5678), parentInode)

	// Add parent cgroup context to cache
	parentCgroup := &model.CGroupContext{
		CGroupID: "parent-cgroup-id",
		CGroupFile: model.PathKey{
			Inode:   parentInode,
			MountID: 123,
		},
	}
	resolver.cgroups.Add(parentInode, parentCgroup)

	// Mock failed direct resolution
	mockFS.On("FindCGroupContext", uint32(1234), uint32(1234)).Return(
		containerutils.ContainerID(""),
		utils.CGroupContext{},
		"",
		errors.New("not found"),
	)

	result := resolver.resolvePidCgroupFallback(process)

	assert.True(t, result)
	assert.Equal(t, containerutils.CGroupID("parent-cgroup-id"), process.CGroup.CGroupID)
	assert.Equal(t, uint32(123), process.CGroup.CGroupFile.MountID)
	assert.Equal(t, parentInode, process.CGroup.CGroupFile.Inode)
	// Note: containerutils.FindContainerID would be called here, but we can't easily mock it
	// in this example as it's a package function

	mockFS.AssertExpectations(t)
}

func TestResolvePidCgroupFallback_SuccessFromParentProc(t *testing.T) {
	resolver, mockFS := createTestResolver(t)
	process := createTestProcess(1234, 5678, "")

	expectedContext := utils.CGroupContext{
		CGroupID:          "parent-cgroup-id",
		CGroupFileMountID: 567,
		CGroupFileInode:   8888,
	}

	// Mock failed direct resolution
	mockFS.On("FindCGroupContext", uint32(1234), uint32(1234)).Return(
		containerutils.ContainerID(""),
		utils.CGroupContext{},
		"",
		errors.New("not found"),
	)

	// Mock successful parent resolution
	mockFS.On("FindCGroupContext", uint32(5678), uint32(5678)).Return(
		containerutils.ContainerID("parent-container-456"),
		expectedContext,
		"/sys/fs/cgroup/parent",
		nil,
	)

	result := resolver.resolvePidCgroupFallback(process)

	assert.True(t, result)
	assert.Equal(t, containerutils.CGroupID("parent-cgroup-id"), process.CGroup.CGroupID)
	assert.Equal(t, uint32(567), process.CGroup.CGroupFile.MountID)
	assert.Equal(t, uint64(8888), process.CGroup.CGroupFile.Inode)
	assert.Equal(t, containerutils.ContainerID("parent-container-456"), process.ContainerID)

	mockFS.AssertExpectations(t)
}

func TestResolvePidCgroupFallback_CompleteFailure(t *testing.T) {
	resolver, mockFS := createTestResolver(t)
	process := createTestProcess(1234, 5678, "")

	// Mock failed direct resolution
	mockFS.On("FindCGroupContext", uint32(1234), uint32(1234)).Return(
		containerutils.ContainerID(""),
		utils.CGroupContext{},
		"",
		errors.New("not found"),
	)

	// Mock failed parent resolution
	mockFS.On("FindCGroupContext", uint32(5678), uint32(5678)).Return(
		containerutils.ContainerID(""),
		utils.CGroupContext{},
		"",
		errors.New("parent not found"),
	)

	result := resolver.resolvePidCgroupFallback(process)

	assert.False(t, result)
	// Process should remain unchanged
	assert.Equal(t, containerutils.CGroupID(""), process.CGroup.CGroupID)
	assert.Equal(t, uint32(0), process.CGroup.CGroupFile.MountID)
	assert.Equal(t, uint64(0), process.CGroup.CGroupFile.Inode)

	mockFS.AssertExpectations(t)
}

func TestResolvePidCgroupFallback_HistoryFoundButCgroupMissing(t *testing.T) {
	resolver, mockFS := createTestResolver(t)
	process := createTestProcess(1234, 5678, "")

	// Add parent to history but not to cgroups cache
	resolver.history.Add(uint32(5678), uint64(9999))

	expectedContext := utils.CGroupContext{
		CGroupID:          "fallback-cgroup-id",
		CGroupFileMountID: 789,
		CGroupFileInode:   7777,
	}

	// Mock failed direct resolution
	mockFS.On("FindCGroupContext", uint32(1234), uint32(1234)).Return(
		containerutils.ContainerID(""),
		utils.CGroupContext{},
		"",
		errors.New("not found"),
	)

	// Mock successful parent proc resolution as fallback
	mockFS.On("FindCGroupContext", uint32(5678), uint32(5678)).Return(
		containerutils.ContainerID("fallback-container-789"),
		expectedContext,
		"/sys/fs/cgroup/fallback",
		nil,
	)

	result := resolver.resolvePidCgroupFallback(process)

	assert.True(t, result)
	assert.Equal(t, containerutils.CGroupID("fallback-cgroup-id"), process.CGroup.CGroupID)
	assert.Equal(t, uint32(789), process.CGroup.CGroupFile.MountID)
	assert.Equal(t, uint64(7777), process.CGroup.CGroupFile.Inode)
	assert.Equal(t, containerutils.ContainerID("fallback-container-789"), process.ContainerID)

	mockFS.AssertExpectations(t)
}

func TestResolvePidCgroupFallback_EmptyCGroupIDIgnored(t *testing.T) {
	resolver, mockFS := createTestResolver(t)
	process := createTestProcess(1234, 5678, "")

	// Mock resolution that returns empty CGroupID (should be ignored)
	mockFS.On("FindCGroupContext", uint32(1234), uint32(1234)).Return(
		containerutils.ContainerID("some-container"),
		utils.CGroupContext{
			CGroupID:          "", // Empty CGroupID
			CGroupFileMountID: 42,
			CGroupFileInode:   9876,
		},
		"/sys/fs/cgroup/test",
		nil,
	)

	// Should fallback to parent resolution
	mockFS.On("FindCGroupContext", uint32(5678), uint32(5678)).Return(
		containerutils.ContainerID(""),
		utils.CGroupContext{},
		"",
		errors.New("parent not found"),
	)

	result := resolver.resolvePidCgroupFallback(process)

	assert.False(t, result)
	mockFS.AssertExpectations(t)
}
