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

func (*fakev3or4EcsClient) GetTask(ctx context.Context) (*v3or4.Task, error) { //nolint:revive // TODO fix revive unused-parameter
	return nil, errors.New("unimplemented")
}

func (store *fakev3or4EcsClient) GetTaskWithTags(ctx context.Context) (*v3or4.Task, error) {
	return store.mockGetTaskWithTags(ctx)
}

func (*fakev3or4EcsClient) GetContainer(ctx context.Context) (*v3or4.Container, error) { //nolint:revive // TODO fix revive unused-parameter
	return nil, errors.New("unimplemented")
}
