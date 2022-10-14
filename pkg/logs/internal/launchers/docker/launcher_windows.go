// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker && windows
// +build docker,windows

package docker

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
)

const (
	basePath = "c:\\programdata\\docker\\containers"
)

// getDockerLogsPath returns the correct path to access the docker logs, taking
// into account the override path
func getDockerLogsPath() string {
	overridePath := coreConfig.Datadog.GetString("logs_config.docker_path_override")
	if len(overridePath) > 0 {
		return overridePath
	}

	return basePath
}

func checkReadAccess() error {
	// We need read access to the docker folder
	path := getDockerLogsPath()

	_, err := ioutil.ReadDir(path)
	return err
}

// getPath returns the file path of the container log to tail.
func getPath(id string) string {
	path := getDockerLogsPath()
	return filepath.Join(path, id, fmt.Sprintf("%s-json.log", id))
}
