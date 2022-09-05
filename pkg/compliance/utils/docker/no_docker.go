// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !docker
// +build !docker

package docker

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
)

// NewDockerClient returns a new docker client
func NewDockerClient() (env.DockerClient, error) {
	return nil, errors.New("docker client requires docker build flag")
}
