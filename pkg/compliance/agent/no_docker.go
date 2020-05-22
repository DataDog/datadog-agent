// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !docker

package agent

import "github.com/DataDog/datadog-agent/pkg/compliance/checks"

var dockerClient = func() checks.DockerClient {
	return nil
}
