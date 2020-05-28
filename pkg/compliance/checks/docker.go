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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func newDockerClient() DockerClient {
	queryTimeout := config.Datadog.GetDuration("docker_query_timeout") * time.Second

	// Major failure risk is here, do that first
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	client, err := docker.ConnectToDocker(ctx)
	if err != nil {
		log.Debugf("no docker client: %v", err)
		return nil
	}
	return client
}
