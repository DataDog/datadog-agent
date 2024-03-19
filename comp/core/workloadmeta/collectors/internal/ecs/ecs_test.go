// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

// ecs collector package
package ecs

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/testutil"
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

func (*fakev3or4EcsClient) GetTask(ctx context.Context) (*v3or4.Task, error) { //nolint:revive // TODO fix revive unused-parameter
	return nil, errors.New("unimplemented")
}

func (store *fakev3or4EcsClient) GetTaskWithTags(ctx context.Context) (*v3or4.Task, error) {
	return store.mockGetTaskWithTags(ctx)
}

func (*fakev3or4EcsClient) GetContainer(ctx context.Context) (*v3or4.Container, error) { //nolint:revive // TODO fix revive unused-parameter
	return nil, errors.New("unimplemented")
}

func TestPull(t *testing.T) {
	entityID := "task1"
	tags := map[string]string{"foo": "bar"}

	tests := []struct {
		name                string
		collectResourceTags bool
		expectedTags        map[string]string
	}{
		{
			name:                "collect tags",
			collectResourceTags: true,
			expectedTags:        tags,
		},
		{
			name:                "don't collect tags",
			collectResourceTags: false,
			expectedTags:        nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := collector{
				resourceTags: make(map[string]resourceTags),
				seen:         make(map[workloadmeta.EntityID]struct{}),
			}

			c.metaV1 = &fakev1EcsClient{
				mockGetTasks: func(ctx context.Context) ([]v1.Task, error) {
					return []v1.Task{
						{
							Arn: entityID,
							Containers: []v1.Container{
								{DockerID: "foo"},
							},
						},
					}, nil
				},
			}
			c.store = &fakeWorkloadmetaStore{}
			c.metaV3or4 = func(metaURI, metaVersion string) v3or4.Client {
				return &fakev3or4EcsClient{
					mockGetTaskWithTags: func(context.Context) (*v3or4.Task, error) {
						return &v3or4.Task{
							TaskTags: map[string]string{
								"foo": "bar",
							},
						}, nil
					},
				}
			}

			c.hasResourceTags = true
			c.collectResourceTags = test.collectResourceTags

			c.Pull(context.TODO())

			taskTags := c.resourceTags[entityID].tags
			assert.Equal(t, taskTags, test.expectedTags)
		})
	}

}

func TestPullWithV4TaskEnabled(t *testing.T) {
	// Start a dummy Http server to simulate ECS metadata endpoints
	// /v1/tasks: return the list of tasks containing datadog-agent task and nginx task
	dummyECS, err := testutil.NewDummyECS(
		testutil.FileHandlerOption("/v4/1234-1/taskWithTags", "./testdata/datadog-agent.json"),
		testutil.FileHandlerOption("/v4/1234-2/taskWithTags", "./testdata/nginx.json"),
		testutil.FileHandlerOption("/v1/tasks", "./testdata/tasks.json"),
	)
	require.Nil(t, err)
	ts := dummyECS.Start()
	defer ts.Close()

	// Add container handler to return the v4 endpoints for different containers
	store := &fakeWorkloadmetaStore{
		getGetContainerHandler: func(id string) (*workloadmeta.Container, error) {
			// datadog-agent container ID, see ./testdata/datadog-agent.json
			if id == "749d28eb7145ff3b6c52b71c59b381c70a884c1615e9f99516f027492679496e" {
				return &workloadmeta.Container{
					EnvVars: map[string]string{
						v3or4.DefaultMetadataURIv4EnvVariable: fmt.Sprintf("%s/v4/1234-1", ts.URL),
					},
				}, nil
			}
			// nginx container ID, see ./testdata/nginx.json
			if id == "2ad9e753a0dfbba1c91e0e7bebaaf3a0918d3ef304b7549b1ced5f573bc05645" {
				return &workloadmeta.Container{
					EnvVars: map[string]string{
						v3or4.DefaultMetadataURIv4EnvVariable: fmt.Sprintf("%s/v4/1234-2", ts.URL),
					},
				}, nil
			}
			return &workloadmeta.Container{
				EnvVars: map[string]string{
					v3or4.DefaultMetadataURIv4EnvVariable: fmt.Sprintf("%s/v4/undefined", ts.URL),
				},
			}, nil
		},
	}

	// create an ECS collector with v4TaskEnabled enabled
	collector := collector{
		store:         store,
		v4TaskEnabled: true,
		metaV1:        v1.NewClient(ts.URL),
		metaV3or4: func(metaURI, metaVersion string) v3or4.Client {
			return v3or4.NewClient(metaURI, metaVersion)
		},
		v4TaskCache:             cache.New(3*time.Minute, 30*time.Second),
		v4TaskRefreshInterval:   3 * time.Minute,
		v4TaskNumberLimitPerRun: 100,
		v4TaskQueue:             make([]string, 0, 5),
		v4TaskRateLimiter:       rate.NewLimiter(rate.Every(1*time.Second/35), 60),
	}

	// Pull 3 times, only the first time should fetch the v4 tasks from the server
	// The other 2 times should fetch the tasks from the cache
	for i := 0; i < 3; i++ {
		store.notifiedEvents = store.notifiedEvents[:0]
		err = collector.Pull(context.Background())
		require.Nil(t, err)
		// two ECS task events and two container events should be notified
		require.Len(t, store.notifiedEvents, 4)

		count := 0
		for _, event := range store.notifiedEvents {
			require.Equal(t, workloadmeta.EventTypeSet, event.Type)
			require.Equal(t, workloadmeta.SourceNodeOrchestrator, event.Source)
			switch entity := event.Entity.(type) {
			case *workloadmeta.ECSTask:
				require.Equal(t, 123457279990, entity.AWSAccountID)
				require.Equal(t, "us-east-1", entity.Region)
				require.Equal(t, "ecs-cluster", entity.ClusterName)
				require.Equal(t, "RUNNING", entity.DesiredStatus)
				require.Equal(t, workloadmeta.ECSLaunchTypeEC2, entity.LaunchType)
				if entity.Family == "datadog-agent" {
					require.Equal(t, "15", entity.Version)
					require.Equal(t, "vpc-123", entity.VPCID)
					count++
				} else if entity.Family == "nginx" {
					require.Equal(t, "3", entity.Version)
					require.Equal(t, "vpc-124", entity.VPCID)
					count++
				} else {
					t.Errorf("unexpected entity family: %s", entity.Family)
				}
			case *workloadmeta.Container:
				require.Equal(t, "RUNNING", entity.KnownStatus)
				require.Equal(t, "HEALTHY", entity.Health.Status)
				if entity.Image.Name == "datadog/datadog-agent" {
					require.Equal(t, "7.50.0", entity.Image.Tag)
					require.Equal(t, "Agent health: PASS", entity.Health.Output)
					count++
				} else if entity.Image.Name == "ghcr.io/nginx/my-nginx" {
					require.Equal(t, "ghcr.io", entity.Image.Registry)
					require.Equal(t, "main", entity.Image.Tag)
					require.Equal(t, "Nginx health: PASS", entity.Health.Output)
					count++
				} else {
					t.Errorf("unexpected image name: %s", entity.Image.Name)
				}
			default:
				t.Errorf("unexpected entity type: %T", entity)
			}
		}
		require.Equal(t, 4, count)
		require.Equal(t, 0, len(collector.v4TaskQueue))
		require.Equal(t, 2, len(collector.v4TaskCache.Items()))
	}
}

func TestGetTaskArnsToFetch(t *testing.T) {
	collector := collector{
		v4TaskCache:             cache.New(3*time.Minute, 30*time.Second),
		v4TaskRefreshInterval:   3 * time.Minute,
		v4TaskNumberLimitPerRun: 10,
		v4TaskQueue:             make([]string, 0, 3),
	}

	// generate tasks
	taskCount := 30
	tasks := make([]v1.Task, 0, taskCount)
	for i := 0; i < taskCount; i++ {
		tasks = append(tasks, v1.Task{
			Arn: fmt.Sprintf("1234-%d", i),
		})
	}

	// add totalTaskInCache tasks to the cache
	taskInCache := 3
	for i := 0; i < taskInCache; i++ {
		collector.v4TaskCache.SetDefault(fmt.Sprintf("1234-%d", i), struct{}{})
	}

	// add stopped tasks to the queue
	stoppedTask := 2
	for i := 0; i < stoppedTask; i++ {
		collector.v4TaskQueue = append(collector.v4TaskQueue, fmt.Sprintf("1234-stopped-%d", i))
	}

	// add running tasks to the queue
	runningTask := 10
	for i := 0; i < runningTask; i++ {
		collector.v4TaskQueue = append(collector.v4TaskQueue, fmt.Sprintf("1234-%d", i))
	}

	taskArns := collector.getTaskArnsToFetch(tasks)

	require.Equal(t, collector.v4TaskNumberLimitPerRun, len(taskArns))
	for i := 3; i < 13; i++ {
		if _, ok := taskArns[fmt.Sprintf("1234-%d", i)]; !ok {
			t.Errorf("task arn %d not found", i)
		}
	}
	assert.Equal(t, 17, len(collector.v4TaskQueue))

}

func TestJitter(t *testing.T) {
	actual := jitter("test")
	assert.Equal(t, 23*time.Second, actual)

	actual = jitter("test2")
	assert.Equal(t, 7*time.Second, actual)
}
