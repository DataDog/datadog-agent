// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

// ecs collector package
package ecs

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	v1 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v1"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
)

type fakeWorkloadmetaStore struct {
	workloadmeta.Component
	notifiedEvents         []workloadmeta.CollectorEvent
	getGetContainerHandler func(id string) (*workloadmeta.Container, error)
}

func (store *fakeWorkloadmetaStore) Notify(events []workloadmeta.CollectorEvent) {
	store.notifiedEvents = append(store.notifiedEvents, events...)
}

func (store *fakeWorkloadmetaStore) GetContainer(id string) (*workloadmeta.Container, error) {
	if store.getGetContainerHandler != nil {
		return store.getGetContainerHandler(id)
	}

	return &workloadmeta.Container{
		EnvVars: map[string]string{
			v3or4.DefaultMetadataURIv4EnvVariable: "fake_uri",
		},
	}, nil
}

type fakev1EcsClient struct {
	mockGetTasks func(context.Context) ([]v1.Task, error)
}

func (c *fakev1EcsClient) GetTasks(ctx context.Context) ([]v1.Task, error) {
	return c.mockGetTasks(ctx)
}

func (c *fakev1EcsClient) GetInstance(_ context.Context) (*v1.Instance, error) {
	return nil, errors.New("unimplemented")
}

type fakev3or4EcsClient struct {
	mockGetTaskWithTags func(context.Context) (*v3or4.Task, error)
}

func (*fakev3or4EcsClient) GetTask(_ context.Context) (*v3or4.Task, error) {
	return nil, errors.New("unimplemented")
}

func (store *fakev3or4EcsClient) GetTaskWithTags(ctx context.Context) (*v3or4.Task, error) {
	return store.mockGetTaskWithTags(ctx)
}

func (*fakev3or4EcsClient) GetContainer(_ context.Context) (*v3or4.Container, error) {
	return nil, errors.New("unimplemented")
}

// TestPull tests the Pull method
func TestPull(t *testing.T) {
	store := &fakeWorkloadmetaStore{}

	mockParser := func(_ context.Context) ([]workloadmeta.CollectorEvent, error) {
		return []workloadmeta.CollectorEvent{
			{
				Type:   workloadmeta.EventTypeSet,
				Source: workloadmeta.SourceRuntime,
				Entity: &workloadmeta.ECSTask{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindECSTask,
						ID:   "test-task",
					},
				},
			},
		}, nil
	}

	collector := &collector{
		store:                store,
		taskCollectionParser: mockParser,
	}

	err := collector.Pull(context.Background())
	require.NoError(t, err)
	require.Len(t, store.notifiedEvents, 1)
	assert.Equal(t, workloadmeta.EventTypeSet, store.notifiedEvents[0].Type)
}

// TestPullError tests Pull method error handling
func TestPullError(t *testing.T) {
	store := &fakeWorkloadmetaStore{}

	mockParser := func(_ context.Context) ([]workloadmeta.CollectorEvent, error) {
		return nil, errors.New("parser error")
	}

	collector := &collector{
		store:                store,
		taskCollectionParser: mockParser,
	}

	err := collector.Pull(context.Background())
	require.Error(t, err)
	assert.Equal(t, "parser error", err.Error())
	require.Len(t, store.notifiedEvents, 0)
}

// TestGetID tests the GetID method
func TestGetID(t *testing.T) {
	collector := &collector{
		id: collectorID,
	}
	assert.Equal(t, collectorID, collector.GetID())
}

// TestGetTargetCatalog tests the GetTargetCatalog method
func TestGetTargetCatalog(t *testing.T) {
	catalog := workloadmeta.NodeAgent | workloadmeta.ProcessAgent
	collector := &collector{
		catalog: catalog,
	}
	assert.Equal(t, catalog, collector.GetTargetCatalog())
}
