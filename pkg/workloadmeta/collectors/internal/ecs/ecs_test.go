// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package ecs

import (
	"context"
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/stretchr/testify/assert"

	v1 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v1"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
)

type fakeWorkloadmetaStore struct {
	workloadmeta.Store
	notifiedEvents []workloadmeta.CollectorEvent
}

func (store *fakeWorkloadmetaStore) Notify(events []workloadmeta.CollectorEvent) {
	store.notifiedEvents = append(store.notifiedEvents, events...)
}

func (store *fakeWorkloadmetaStore) GetContainer(id string) (*workloadmeta.Container, error) {
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

func (c *fakev1EcsClient) GetInstance(ctx context.Context) (*v1.Instance, error) {
	return nil, errors.New("unimplemented")
}

type fakev3or4EcsClient struct {
	mockGetTaskWithTags func(context.Context) (*v3or4.Task, error)
}

func (*fakev3or4EcsClient) GetTask(ctx context.Context) (*v3or4.Task, error) {
	return nil, errors.New("unimplemented")
}

func (c *fakev3or4EcsClient) GetTaskWithTags(ctx context.Context) (*v3or4.Task, error) {
	return c.mockGetTaskWithTags(ctx)
}

func (*fakev3or4EcsClient) GetContainer(ctx context.Context) (*v3or4.Container, error) {
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
