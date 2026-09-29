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
	"github.com/stretchr/testify/require"

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

	cacheEntry := resolver.resolveFromFallback(1234)
	assert.NotNil(t, cacheEntry)
	assert.Equal(t, containerutils.CGroupID("test-cgroup-id"), cacheEntry.GetCGroupID())
	assert.Equal(t, uint64(9876), cacheEntry.GetCGroupInode())
	assert.Equal(t, containerutils.ContainerID("container-123"), cacheEntry.GetContainerID())

	mockFS.AssertExpectations(t)
}

// podUIDTestCases covers the cgroup layouts the agent sees in production. The
// systemd driver (the Kubernetes default) escapes the dashes of the pod UID as
// underscores, because "-" is its own slice hierarchy separator; the pod UID
// must be un-escaped back to the canonical dashed form so that it matches what
// the Kubernetes API, the audit logs, and /var/lib/kubelet/pods/<uid>/ use.
var podUIDTestCases = []struct {
	name        string
	cgroupID    containerutils.CGroupID
	containerID containerutils.ContainerID
	podUID      string
}{
	{
		name:        "cgroupfs driver, dashed pod UID",
		cgroupID:    "/kubepods/besteffort/pod48d25824-cbe2-4fdc-9928-5bb49e05473d/cri-containerd-c40dff48f1d53c3f07a50aa12bb9ae0e58c0927dc6b1d77e3f166784722642ad.scope",
		containerID: "c40dff48f1d53c3f07a50aa12bb9ae0e58c0927dc6b1d77e3f166784722642ad",
		podUID:      "48d25824-cbe2-4fdc-9928-5bb49e05473d",
	},
	{
		name:        "systemd driver, underscore-escaped pod UID",
		cgroupID:    "/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod48d25824_cbe2_4fdc_9928_5bb49e05473d.slice/cri-containerd-c40dff48f1d53c3f07a50aa12bb9ae0e58c0927dc6b1d77e3f166784722642ad.scope",
		containerID: "c40dff48f1d53c3f07a50aa12bb9ae0e58c0927dc6b1d77e3f166784722642ad",
		podUID:      "48d25824-cbe2-4fdc-9928-5bb49e05473d",
	},
	{
		name:        "systemd driver, guaranteed QoS class",
		cgroupID:    "/kubepods.slice/kubepods-pod48d25824_cbe2_4fdc_9928_5bb49e05473d.slice/cri-containerd-c40dff48f1d53c3f07a50aa12bb9ae0e58c0927dc6b1d77e3f166784722642ad.scope",
		containerID: "c40dff48f1d53c3f07a50aa12bb9ae0e58c0927dc6b1d77e3f166784722642ad",
		podUID:      "48d25824-cbe2-4fdc-9928-5bb49e05473d",
	},
	{
		// the pod sandbox/pause cgroup has no container ID segment: the pod UID
		// is then the only workload identifier we have.
		name:     "systemd driver, pod cgroup without a container ID",
		cgroupID: "/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod48d25824_cbe2_4fdc_9928_5bb49e05473d.slice",
		podUID:   "48d25824-cbe2-4fdc-9928-5bb49e05473d",
	},
	{
		name:     "cgroupfs driver, pod cgroup without a container ID",
		cgroupID: "/kubepods/besteffort/pod48d25824-cbe2-4fdc-9928-5bb49e05473d",
		podUID:   "48d25824-cbe2-4fdc-9928-5bb49e05473d",
	},
	{
		name:     "host cgroup, no pod UID and no container ID",
		cgroupID: "/user.slice/user-1000.slice/user@1000.service/apps.slice/apps-org.gnome.Terminal.slice/vte-spawn-f9176c6a-2a34-4ce2-86af-60d16888ed8e.scope",
	},
}

// TestAddCGroup_PodUID exercises the event resolution path, the one used when
// the cgroup comes from an eBPF event.
func TestAddCGroup_PodUID(t *testing.T) {
	for i, tc := range podUIDTestCases {
		t.Run(tc.name, func(t *testing.T) {
			resolver, _ := createTestResolver(t)

			cacheEntry := resolver.Add(model.CGroupContext{
				CGroupID: tc.cgroupID,
				CGroupPathKey: model.PathKey{
					MountID: 42,
					Inode:   uint64(9876 + i),
				},
				CGroupSource: model.CGroupSourceEvent,
			})

			require.NotNil(t, cacheEntry)
			assert.Equal(t, tc.cgroupID, cacheEntry.GetCGroupID())
			assert.Equal(t, tc.containerID, cacheEntry.GetContainerID())
			assert.Equal(t, tc.podUID, cacheEntry.GetContainerContext().PodUID)
		})
	}
}

// TestResolvePidCgroupFallback_PodUID exercises the procfs fallback path, the
// one used at snapshot time and when the event carries no usable cgroup.
func TestResolvePidCgroupFallback_PodUID(t *testing.T) {
	for i, tc := range podUIDTestCases {
		t.Run(tc.name, func(t *testing.T) {
			resolver, mockFS := createTestResolver(t)

			pid := uint32(1234 + i)
			mockFS.On("FindCGroupContext", pid, pid).Return(
				tc.containerID,
				utils.CGroupContext{
					CGroupID:          tc.cgroupID,
					CGroupFileMountID: 42,
					CGroupFileInode:   uint64(9876 + i),
				},
				"/sys/fs/cgroup/test",
				nil,
			)

			cacheEntry := resolver.resolveFromFallback(pid)
			require.NotNil(t, cacheEntry)
			assert.Equal(t, tc.cgroupID, cacheEntry.GetCGroupID())
			assert.Equal(t, tc.containerID, cacheEntry.GetContainerID())
			assert.Equal(t, tc.podUID, cacheEntry.GetContainerContext().PodUID)

			mockFS.AssertExpectations(t)
		})
	}
}

// TestAddCGroup_PodOnlyContextIsNotAContainer pins down the deliberate
// asymmetry introduced with the pod UID: a pod cgroup with no container ID
// carries a pod UID, but is still not a container as far as IsNull() and the
// container cache are concerned, since both key off the container ID.
func TestAddCGroup_PodOnlyContextIsNotAContainer(t *testing.T) {
	resolver, _ := createTestResolver(t)

	cacheEntry := resolver.Add(model.CGroupContext{
		CGroupID:      "/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod48d25824_cbe2_4fdc_9928_5bb49e05473d.slice",
		CGroupPathKey: model.PathKey{MountID: 42, Inode: 4242},
		CGroupSource:  model.CGroupSourceEvent,
	})

	require.NotNil(t, cacheEntry)
	assert.Equal(t, "48d25824-cbe2-4fdc-9928-5bb49e05473d", cacheEntry.GetContainerContext().PodUID)
	assert.Empty(t, cacheEntry.GetContainerID())
	assert.True(t, cacheEntry.IsContainerContextNull())

	assert.Nil(t, resolver.GetCacheEntryContainerID(""), "a pod-only entry must not be registered in the container cache")
	assert.NotNil(t, resolver.GetCacheEntryByCgroupID(cacheEntry.GetCGroupID()), "a pod-only entry is tracked as a host cgroup")
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

	cacheEntry := resolver.resolveFromFallback(1234)
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

	cacheEntry := resolver.resolveFromFallback(1234)
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

	cacheEntry = resolver.resolveFromFallback(5678)
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

	cacheEntry = resolver.AddPID(1234, model.CGroupContext{})

	assert.NotNil(t, cacheEntry)
	assert.Equal(t, containerutils.CGroupID("fallback-cgroup-id"), cacheEntry.GetCGroupID())

	mockFS.AssertExpectations(t)
}
