// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

// Package docker provides a Docker client.
package docker

import (
	"context"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// Client defines the interface of our custom Docker client (e.g. DockerUtil)
type Client interface {
	RawClient() *client.Client
	RawContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error)
	ResolveImageName(ctx context.Context, image string) (string, error)
	Images(ctx context.Context, includeIntermediate bool) ([]image.Summary, error)
	GetPreferredImageName(imageID string, repoTags []string, repoDigests []string) string
	GetStorageStats(ctx context.Context) ([]*StorageStats, error)
	CountVolumes(ctx context.Context) (int, int, error)
	LatestContainerEvents(ctx context.Context, since time.Time, filter *containers.Filter) ([]*ContainerEvent, time.Time, error)
}
