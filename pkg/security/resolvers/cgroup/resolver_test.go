// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroup

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// MockCGroupFS implements a mock for FSInterface
type MockCGroupFS struct {
	mock.Mock
}

func (m *MockCGroupFS) FindCGroupContext(tgid, pid uint32) (containerutils.ContainerID, utils.CGroupContext, string, error) {
	args := m.Called(tgid, pid)
	return args.Get(0).(containerutils.ContainerID), args.Get(1).(utils.CGroupContext), args.String(2), args.Error(3)
}

func (m *MockCGroupFS) GetCGroupPids(cgroupID string) ([]uint32, error) {
	args := m.Called(cgroupID)
	return args.Get(0).([]uint32), args.Error(1)
}

// createTestResolver creates a resolver with mocked dependencies for testing
func createTestResolver(t *testing.T) (*Resolver, *MockCGroupFS) {
	mockCGroupFS := &MockCGroupFS{}

	resolver, err := NewResolver(nil, mockCGroupFS, nil)
	assert.NoError(t, err)

	return resolver, mockCGroupFS
}

func TestResolvePidCgroupFallback_SuccessDirectResolution(t *testing.T) {
	resolver, mockFS := createTestResolver(t)

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

	cacheEntry := resolver.resolveFromFallback(1234, 5678, time.Now())
	assert.NotNil(t, cacheEntry)
	assert.Equal(t, containerutils.CGroupID("test-cgroup-id"), cacheEntry.GetCGroupID())
	assert.Equal(t, uint64(9876), cacheEntry.GetCGroupInode())
	assert.Equal(t, containerutils.ContainerID("container-123"), cacheEntry.GetContainerID())

	mockFS.AssertExpectations(t)
}

func TestResolvePidCgroupFallback_FailInvalidPPid(t *testing.T) {
	resolver, mockFS := createTestResolver(t)

	// Test case 1: PPid equals Pid
	mockFS.On("FindCGroupContext", uint32(1234), uint32(1234)).Return(
		containerutils.ContainerID(""),
		utils.CGroupContext{},
		"",
		errors.New("not found"),
	)

	cacheEntry := resolver.resolveFromFallback(1234, 1234, time.Now())
	assert.Nil(t, cacheEntry)

	// Test case 2: PPid is 0
	mockFS.On("FindCGroupContext", uint32(1234), uint32(1234)).Return(
		containerutils.ContainerID(""),
		utils.CGroupContext{},
		"",
		errors.New("not found"),
	)

	cacheEntry = resolver.resolveFromFallback(1234, 0, time.Now())
	assert.Nil(t, cacheEntry)

	mockFS.AssertExpectations(t)
}

func TestResolvePidCgroupFallback_SuccessFromHistory(t *testing.T) {
	resolver, mockFS := createTestResolver(t)

	ppid := uint32(5678)
	parentPathKey := model.PathKey{Inode: 9999}

	// Add parent cgroup to history
	resolver.history.Add(ppid, parentPathKey.Inode)

	// Add parent cgroup context to cache
	parentCgroupContext := model.CGroupContext{
		CGroupID:      "parent-cgroup-id",
		CGroupPathKey: parentPathKey,
	}
	cacheEntry := resolver.AddPID(1234, 5678, time.Now(), parentCgroupContext)
	assert.NotNil(t, cacheEntry)
	assert.NotNil(t, cacheEntry.GetCGroupContext().Releasable)

	// Mock failed direct resolution
	mockFS.On("FindCGroupContext", uint32(1234), uint32(1234)).Return(
		containerutils.ContainerID(""),
		utils.CGroupContext{},
		"",
		errors.New("not found"),
	)

	cacheEntry = resolver.resolveFromFallback(1234, 5678, time.Now())
	assert.NotNil(t, cacheEntry)
	assert.Equal(t, containerutils.CGroupID("parent-cgroup-id"), cacheEntry.GetCGroupID())
	assert.Equal(t, parentPathKey, cacheEntry.GetCGroupContext().CGroupPathKey)
	// Note: containerutils.FindContainerID would be called here, but we can't easily mock it
	// in this example as it's a package function

	mockFS.AssertExpectations(t)
}

func TestResolvePidCgroupFallback_SuccessFromParentProc(t *testing.T) {
	resolver, mockFS := createTestResolver(t)

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

	cacheEntry := resolver.resolveFromFallback(1234, 5678, time.Now())
	assert.NotNil(t, cacheEntry)
	assert.Equal(t, containerutils.CGroupID("parent-cgroup-id"), cacheEntry.GetCGroupID())
	assert.Equal(t, expectedContext.CGroupFileInode, cacheEntry.GetCGroupInode())
	assert.Equal(t, containerutils.ContainerID("parent-container-456"), cacheEntry.GetContainerID())

	mockFS.AssertExpectations(t)
}

func TestResolvePidCgroupFallback_CompleteFailure(t *testing.T) {
	resolver, mockFS := createTestResolver(t)

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

	cacheEntry := resolver.resolveFromFallback(1234, 5678, time.Now())
	assert.Nil(t, cacheEntry)

	mockFS.AssertExpectations(t)
}

func TestResolvePidCgroupFallback_HistoryFoundButCGroupMissing(t *testing.T) {
	resolver, mockFS := createTestResolver(t)

	// Add parent to history but not to cgroups cache
	resolver.history.Add(uint32(5678), 9999)

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

	cacheEntry := resolver.resolveFromFallback(1234, 5678, time.Now())
	assert.NotNil(t, cacheEntry)
	assert.Equal(t, containerutils.CGroupID("fallback-cgroup-id"), cacheEntry.GetCGroupID())
	assert.Equal(t, expectedContext.CGroupFileInode, cacheEntry.GetCGroupInode())
	assert.Equal(t, containerutils.ContainerID("fallback-container-789"), cacheEntry.GetContainerID())

	mockFS.AssertExpectations(t)
}

func TestResolvePidCgroupFallback_EmptyCGroupIDIgnored(t *testing.T) {
	resolver, mockFS := createTestResolver(t)

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

	cacheEntry := resolver.resolveFromFallback(1234, 5678, time.Now())
	assert.Nil(t, cacheEntry)

	mockFS.AssertExpectations(t)
}

func TestResolvePidCgroupFallback_UpdateExistingCacheEntry(t *testing.T) {
	resolver, mockFS := createTestResolver(t)

	// Mock resolution that returns empty CGroupID (should be ignored)
	mockFS.On("FindCGroupContext", uint32(1234), uint32(1234)).Return(
		containerutils.ContainerID("some-container"),
		utils.CGroupContext{
			CGroupID:          "fallback-cgroup-id-success", // Empty CGroupID
			CGroupFileMountID: 42,
			CGroupFileInode:   9876,
		},
		"/sys/fs/cgroup/test",
		nil,
	)

	cacheEntry := resolver.resolveFromFallback(1234, 9999, time.Now())
	assert.NotNil(t, cacheEntry)

	// Mock resolution that returns empty CGroupID (should be ignored)
	mockFS.On("FindCGroupContext", uint32(5678), uint32(5678)).Return(
		containerutils.ContainerID("some-container"),
		utils.CGroupContext{
			CGroupID:          "fallback-cgroup-id-fail", // Empty CGroupID
			CGroupFileMountID: 42,
			CGroupFileInode:   9876,
		},
		"/sys/fs/cgroup/test",
		nil,
	)

	cacheEntry = resolver.resolveFromFallback(5678, 9999, time.Now())
	assert.NotNil(t, cacheEntry)
	assert.Equal(t, containerutils.CGroupID("fallback-cgroup-id-success"), cacheEntry.GetCGroupID())

	mockFS.AssertExpectations(t)
}

func TestResolveForceFallbackIfCGroupIsNull(t *testing.T) {
	resolver, mockFS := createTestResolver(t)

	cacheEntry := cgroupModel.NewCacheEntry(model.ContainerContext{
		ContainerID: "some-container",
	}, model.CGroupContext{
		CGroupID: "fallback-cgroup-id-fail",
	}, 1234)

	// add an empty entry to the cache
	resolver.cacheEntriesByPathKey.Add(0, cacheEntry)

	// Mock resolution that returns empty CGroupID (should be ignored)
	mockFS.On("FindCGroupContext", uint32(1234), uint32(1234)).Return(
		containerutils.ContainerID("some-container"),
		utils.CGroupContext{
			CGroupID:          "fallback-cgroup-id", // Empty CGroupID
			CGroupFileMountID: 42,
			CGroupFileInode:   9876,
		},
		"/sys/fs/cgroup/test",
		nil,
	)

	cacheEntry = resolver.AddPID(1234, 5678, time.Now(), model.CGroupContext{})

	assert.NotNil(t, cacheEntry)
	assert.Equal(t, containerutils.CGroupID("fallback-cgroup-id"), cacheEntry.GetCGroupID())

	mockFS.AssertExpectations(t)
}
