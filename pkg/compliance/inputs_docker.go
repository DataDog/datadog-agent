// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package compliance

import (
	"context"

	dockerutil "github.com/DataDog/datadog-agent/pkg/util/docker"
	docker "github.com/docker/docker/client"
)

func newDockerClient(ctx context.Context) (docker.APIClient, error) {
	cl, err := dockerutil.ConnectToDocker(ctx)
	if err != nil {
		return nil, ErrIncompatibleEnvironment
	}
	return cl, nil
}
