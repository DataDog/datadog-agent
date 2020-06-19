// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build docker

package checks

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

func newDockerClient() (DockerClient, error) {
	queryTimeout := config.Datadog.GetDuration("docker_query_timeout") * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	return docker.ConnectToDocker(ctx)
}
