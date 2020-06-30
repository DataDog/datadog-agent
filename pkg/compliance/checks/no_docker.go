// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !docker

package checks

import "errors"

func newDockerClient() (DockerClient, error) {
	return nil, errors.New("docker client requires docker build flag")
}
