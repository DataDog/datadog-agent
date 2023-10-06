// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd && (windows || linux)

package containerd

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/containerd/fake"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/stretchr/testify/assert"
)

type mockedContainer struct {
	containerd.Container

	id string
}

func (mc mockedContainer) ID() string {
	return mc.id
}

func TestGetContainerIDForPID(t *testing.T) {
	pidMap := map[string][]containerd.ProcessInfo{
		"cID1": {containerd.ProcessInfo{Pid: 10}},
	}

	fakeClient := fake.MockedContainerdClient{
		MockNamespaces: func(ctx context.Context) ([]string, error) {
			return []string{"ns"}, nil
		},
		MockContainers: func(namespace string) ([]containerd.Container, error) {
			return []containerd.Container{
				mockedContainer{id: "cID1"},
				mockedContainer{id: "cID2"},
			}, nil
		},
		MockTaskPids: func(namespace string, ctn containerd.Container) ([]containerd.ProcessInfo, error) {
			return pidMap[ctn.ID()], nil
		},
	}

	collector := containerdCollector{
		client:   &fakeClient,
		pidCache: provider.NewCache(pidCacheGCInterval),
	}

	// Cache is empty, will trigger a full refresh
	cID1, err := collector.GetContainerIDForPID(10, time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, "cID1", cID1)

	// Add an entry for PID 20, should not be picked up because full refresh is recent enough
	pidMap["cID2"] = []containerd.ProcessInfo{{Pid: 20}}
	cID2, err := collector.GetContainerIDForPID(20, time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, "", cID2)

	cID2, err = collector.GetContainerIDForPID(20, 0)
	assert.NoError(t, err)
	assert.Equal(t, "cID2", cID2)
}

// Returns a fake containerd client for testing.
// For these tests we need 2 things:
//   - 1) Being able to control the metrics returned by the TaskMetrics
//     function.
//   - 2) Define functions like Info, Spec, etc. so they don't return errors.
func containerdClient(metrics *types.Metric) *fake.MockedContainerdClient {
	return &fake.MockedContainerdClient{
		MockTaskMetrics: func(namespace string, ctn containerd.Container) (*types.Metric, error) {
			return metrics, nil
		},
		MockContainer: func(namespace string, id string) (containerd.Container, error) {
			return mockedContainer{}, nil
		},
		MockInfo: func(namespace string, ctn containerd.Container) (containers.Container, error) {
			return containers.Container{}, nil
		},
		MockSpec: func(namespace string, ctn containers.Container) (*oci.Spec, error) {
			return nil, nil
		},
		MockTaskPids: func(namespace string, ctn containerd.Container) ([]containerd.ProcessInfo, error) {
			return nil, nil
		},
	}
}
