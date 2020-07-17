// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

// +build !docker

package metadata

import (
	"github.com/DataDog/datadog-agent/pkg/util/docker"

	v1 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v1"
	v2 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2"
	v3 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3"
)

// V1 returns a client for the ECS metadata API v1, also called introspection
// endpoint, by detecting the endpoint address. Returns an error if it was not
// possible to detect the endpoint address.
func V1() (*v1.Client, error) {
	return nil, docker.ErrDockerNotCompiled
}

// V2 returns a client for the ECS metadata API v2 that uses the default
// endpoint address.
func V2() *v2.Client {
	return v2.NewDefaultClient()
}

// V3 returns a client for the ECS metadata API v3 by detecting the endpoint
// address for the specified container. Returns an error if it was not possible
// to detect the endpoint address.
func V3(containerID string) (*v3.Client, error) {
	return nil, docker.ErrDockerNotCompiled
}

// V3FromCurrentTask returns a client for the ECS metadata API v3 by detedting
// the endpoint address from the task the executable is running in. Returns an
// error if it was not possible to detect the endpoint address.
func V3FromCurrentTask() (*v3.Client, error) {
	return nil, docker.ErrDockerNotCompiled
}
