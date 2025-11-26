// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package containers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go4.org/intern"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/events"
)

func mockReadContainerItemResult(resolvConf network.ResolvConf) readContainerItemResult {
	item := containerStoreItem{
		timestamp:  time.Now(),
		resolvConf: resolvConf,
	}
	return readContainerItemResult{item: item}
}

func mockReadContainerItem(resolvConfs map[network.ContainerID]network.ResolvConf) func(context.Context, *events.Process) (readContainerItemResult, error) {
	return func(_ context.Context, entry *events.Process) (readContainerItemResult, error) {
		return mockReadContainerItemResult(resolvConfs[entry.ContainerID]), nil
	}
}

func mockConnection(sourceID *intern.Value) network.ConnectionStats {
	return network.ConnectionStats{
		ContainerID: struct {
			Source, Dest *intern.Value
		}{Source: sourceID},
	}
}

func TestContainerStoreMapping(t *testing.T) {
	t.Parallel()
	// basic test of container store behavior
	cs, err := NewContainerStore(10)
	require.NoError(t, err)
	defer cs.Stop()

	containerID := intern.GetByString("container-1")
	processEvent := &events.Process{
		Pid:         12345,
		ContainerID: containerID,
		StartTime:   time.Now().UnixNano(),
	}

	resolvConf := stringInterner.GetString("nameserver 8.8.8.8\nnameserver 8.8.4.4")
	mockMap := map[network.ContainerID]network.ResolvConf{
		containerID: resolvConf,
	}
	cs.readContainerItem = mockReadContainerItem(mockMap)

	initialSize := cs.cache.Len()
	require.Equal(t, 0, initialSize, "Cache should start empty")

	cs.HandleProcessEvent(processEvent)

	require.Eventually(t, func() bool {
		return cs.cache.Len() == 1
	}, 2*time.Second, 50*time.Millisecond, "Cache size should increase to 1 after adding a process")

	// if a connection is there, it should get mapped
	resolveConfs := cs.GetResolvConfMap([]network.ConnectionStats{
		mockConnection(containerID),
		mockConnection(intern.GetByString("unrelated-connection-1")),
	})

	require.Equal(t, mockMap, resolveConfs)

	// unrelated connections should not get mapped
	resolveConfs = cs.GetResolvConfMap([]network.ConnectionStats{
		mockConnection(intern.GetByString("unrelated-connection-1")),
	})
	require.Empty(t, resolveConfs)
}

func TestContainerStoreErrorHandling(t *testing.T) {
	t.Parallel()
	// checks that an error causes the cache to be populated (so it doesn't spam attempts to read procfs)
	cs, err := NewContainerStore(10)
	require.NoError(t, err)
	defer cs.Stop()

	containerID := intern.GetByString("container-1")
	processEvent := &events.Process{
		Pid:         12345,
		ContainerID: containerID,
		StartTime:   time.Now().UnixNano(),
	}

	// mock an error result
	cs.readContainerItem = func(_ context.Context, _ *events.Process) (readContainerItemResult, error) {
		return readContainerItemResult{}, errors.New("failed to read container item")
	}

	cs.HandleProcessEvent(processEvent)

	require.Eventually(t, func() bool {
		return cs.cache.Len() == 1
	}, 2*time.Second, 50*time.Millisecond, "Cache size should increase to 1 after error")

	item, ok := cs.cache.Get(containerID)
	require.True(t, ok, "Container should be in cache after error")
	require.Nil(t, item.resolvConf, "ResolvConf should be empty after error")
	require.False(t, item.timestamp.IsZero(), "Timestamp should be set")

	// since this is an error entry, there should be no data
	resolveConfs := cs.GetResolvConfMap([]network.ConnectionStats{
		mockConnection(containerID),
	})
	require.Empty(t, resolveConfs, "GetResolvConfMap should not return for error entry")
}

func TestContainerStoreNoData(t *testing.T) {
	t.Parallel()
	// makes sure that the "no data" case doesn't populate the cache
	cs, err := NewContainerStore(10)
	require.NoError(t, err)
	defer cs.Stop()

	containerID := intern.GetByString("container-1")
	processEvent := &events.Process{
		Pid:         12345,
		ContainerID: containerID,
		StartTime:   time.Now().UnixNano(),
	}

	callCount := 0
	resolvConf := stringInterner.GetString("nameserver 1.1.1.1\nnameserver 1.0.0.1")
	cs.readContainerItem = func(_ context.Context, _ *events.Process) (readContainerItemResult, error) {
		callCount++
		switch callCount {
		case 1:
			return readContainerItemResult{noDataReason: "process not running"}, nil
		case 2:
			return mockReadContainerItemResult(resolvConf), nil
		default:
			require.Fail(t, "Should only be called twice")
			return readContainerItemResult{}, errors.New("shouldn't get here")
		}
	}

	// send two events, first one should be dropped
	cs.HandleProcessEvent(processEvent)
	cs.HandleProcessEvent(processEvent)

	require.Eventually(t, func() bool {
		return cs.cache.Len() == 1
	}, 2*time.Second, 50*time.Millisecond, "Cache size should increase to 1 after valid data")

	require.Equal(t, 2, callCount, "readContainerItem should be called twice")

	item, ok := cs.cache.Get(containerID)
	require.True(t, ok, "Container should be in cache")
	require.Equal(t, resolvConf, item.resolvConf, "ResolvConf should match")
}
func TestContainerStoreDuplicate(t *testing.T) {
	t.Parallel()
	// makes sure that nothing weird happens when you repeat a process entry
	cs, err := NewContainerStore(10)
	require.NoError(t, err)
	defer cs.Stop()

	containerID := intern.GetByString("container-1")
	processEvent := &events.Process{
		Pid:         12345,
		ContainerID: containerID,
		StartTime:   time.Now().UnixNano(),
	}

	callCount := 0
	resolvConf := stringInterner.GetString("nameserver 1.1.1.1\nnameserver 1.0.0.1")
	cs.readContainerItem = func(_ context.Context, _ *events.Process) (readContainerItemResult, error) {
		callCount++
		switch callCount {
		case 1:
			return mockReadContainerItemResult(resolvConf), nil
		default:
			require.Fail(t, "Should only be called once")
			return readContainerItemResult{}, errors.New("shouldn't get here")
		}
	}

	// send two events, second one should be dropped
	cs.HandleProcessEvent(processEvent)
	cs.HandleProcessEvent(processEvent)

	require.Eventually(t, func() bool {
		return cs.cache.Len() == 1
	}, 2*time.Second, 50*time.Millisecond, "Cache size should increase to 1 after valid data")

	require.Equal(t, 1, callCount, "readContainerItem should be called once")

	item, ok := cs.cache.Get(containerID)
	require.True(t, ok, "Container should be in cache")
	require.Equal(t, resolvConf, item.resolvConf, "ResolvConf should match")
}
