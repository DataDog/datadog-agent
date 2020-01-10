// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !docker

package legacy

import (
	"fmt"
)

// ImportDockerConf is a place holder if the agent is built without the docker flag
func ImportDockerConf(src, dst string, overwrite bool) error {
	fmt.Println("This agent was build without docker support: could not convert docker_daemon.yaml")
	return nil
}
