// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package docker

import (
	"errors"
)

var (
	// ErrDockerNotAvailable is returned if Docker is not running on the current machine.
	// We'll use this when configuring the DockerUtil so we don't error on non-docker machines.
	ErrDockerNotAvailable = errors.New("docker not available")

	// ErrDockerNotCompiled is returned if docker support is not compiled in.
	// User classes should handle that case as gracefully as possible.
	ErrDockerNotCompiled = errors.New("docker support not compiled in")
)
