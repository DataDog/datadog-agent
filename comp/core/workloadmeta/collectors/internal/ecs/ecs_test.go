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

	"github.com/DataDog/datadog-agent/comp/core/config"
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

func (*fakev3or4EcsClient) GetContainerStats(_ context.Context, _ string) (*v3or4.ContainerStatsV4, error) {
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

// TestDetectLaunchType tests the detectLaunchType method with AWS_EXECUTION_ENV
func TestDetectLaunchType(t *testing.T) {
	tests := []struct {
		name            string
		awsExecutionEnv string
		expectedType    workloadmeta.ECSLaunchType
	}{
		{
			name:            "AWS_EXECUTION_ENV_FARGATE",
			awsExecutionEnv: "AWS_ECS_FARGATE",
			expectedType:    workloadmeta.ECSLaunchTypeFargate,
		},
		{
			name:            "AWS_EXECUTION_ENV_MANAGED_INSTANCES",
			awsExecutionEnv: "AWS_ECS_MANAGED_INSTANCES",
			expectedType:    workloadmeta.ECSLaunchTypeManagedInstances,
		},
		{
			name:            "AWS_EXECUTION_ENV_EC2",
			awsExecutionEnv: "AWS_ECS_EC2",
			expectedType:    workloadmeta.ECSLaunchTypeEC2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			t.Setenv("AWS_EXECUTION_ENV", tt.awsExecutionEnv)

			collector := &collector{
				deploymentMode: deploymentModeDaemon,
			}

			result := collector.detectLaunchType(context.Background())
			assert.Equal(t, tt.expectedType, result, "Launch type detection failed for %s", tt.name)
		})
	}
}

// TestDetermineDeploymentMode tests the determineDeploymentMode method
func TestDetermineDeploymentMode(t *testing.T) {
	tests := []struct {
		name         string
		configValue  string
		setupEnv     func(*testing.T)
		expectedMode deploymentMode
	}{
		{
			name:         "Explicit daemon mode on EC2",
			configValue:  "daemon",
			setupEnv:     func(t *testing.T) { t.Setenv("AWS_EXECUTION_ENV", "AWS_ECS_EC2") },
			expectedMode: deploymentModeDaemon,
		},
		{
			name:         "Explicit sidecar mode on Managed Instances",
			configValue:  "sidecar",
			setupEnv:     func(t *testing.T) { t.Setenv("AWS_EXECUTION_ENV", "AWS_ECS_MANAGED_INSTANCES") },
			expectedMode: deploymentModeSidecar,
		},
		{
			name:         "Auto mode - EC2 defaults to daemon",
			configValue:  "auto",
			setupEnv:     func(t *testing.T) { t.Setenv("AWS_EXECUTION_ENV", "AWS_ECS_EC2") },
			expectedMode: deploymentModeDaemon,
		},
		{
			name:         "Auto mode - Managed Instances defaults to daemon",
			configValue:  "auto",
			setupEnv:     func(t *testing.T) { t.Setenv("AWS_EXECUTION_ENV", "AWS_ECS_MANAGED_INSTANCES") },
			expectedMode: deploymentModeDaemon,
		},
		{
			name:         "Auto mode - Fargate defaults to sidecar",
			configValue:  "auto",
			setupEnv:     func(t *testing.T) { t.Setenv("ECS_FARGATE", "true") },
			expectedMode: deploymentModeSidecar,
		},
		{
			name:         "Unknown mode defaults to daemon",
			configValue:  "invalid",
			setupEnv:     func(_ *testing.T) {},
			expectedMode: deploymentModeDaemon,
		},
		{
			name:         "Invalid: Fargate + daemon mode → auto-corrects to sidecar",
			configValue:  "daemon",
			setupEnv:     func(t *testing.T) { t.Setenv("ECS_FARGATE", "true") },
			expectedMode: deploymentModeSidecar,
		},
		{
			name:         "Invalid: EC2 + sidecar mode → auto-corrects to daemon",
			configValue:  "sidecar",
			setupEnv:     func(t *testing.T) { t.Setenv("AWS_EXECUTION_ENV", "AWS_ECS_EC2") },
			expectedMode: deploymentModeDaemon,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv(t)

			// Create a mock config with the test value
			mockConfig := config.NewMockWithOverrides(t, map[string]interface{}{
				"ecs_deployment_mode": tt.configValue,
			})
			collector := &collector{
				config: mockConfig,
			}

			result := collector.determineDeploymentMode()
			assert.Equal(t, tt.expectedMode, result, "Deployment mode detection failed for %s", tt.name)
		})
	}
}
