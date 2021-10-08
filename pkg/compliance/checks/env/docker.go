// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package env

import (
	"context"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

// DockerClient abstracts Docker API client
type DockerClient interface {
	client.ConfigAPIClient
	client.ContainerAPIClient
	client.ImageAPIClient
	client.NodeAPIClient
	client.NetworkAPIClient
	client.SystemAPIClient
	client.VolumeAPIClient
	ServerVersion(ctx context.Context) (types.Version, error)
	Close() error
}
