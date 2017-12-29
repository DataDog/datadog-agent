// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package config

import (
	"os"
	"path"

	log "github.com/cihub/seelog"
)

// AutoAddListeners adds the docker listener if needed
// TODO: support more listeners (kubelet, ...)
func AutoAddListeners(listeners []Listeners) []Listeners {
	autoAdd := false
	for _, l := range listeners {
		switch l.Name {
		case "docker":
			return listeners
		case "auto":
			autoAdd = true
		}
	}
	if autoAdd == false || isDockerRun() == false {
		return listeners
	}
	dl := Listeners{Name: "docker"}
	log.Infof("auto adding %q listener", dl.Name)
	return append(listeners, dl)
}

// isDockerSocketPresent checks if the docker socket is present on the fs
func isDockerSocketPresent() bool {
	const dockerSocket = "docker.sock"

	for _, directory := range []string{"/var/run/", "/run"} {
		socketPath := path.Join(directory, dockerSocket)
		st, err := os.Stat(socketPath)
		if err != nil {
			continue
		}
		if (st.Mode() & os.ModeSocket) != 0 {
			log.Debugf("found docker socket: %s", socketPath)
			return true
		}
	}
	return false
}

// isDockerHostEnv checks if the DOCKER_HOST environment variable is set
func isDockerHostEnv() bool {
	return os.Getenv("DOCKER_HOST") != ""
}

// isDockerRun check with several options to determine if docker runs
func isDockerRun() bool {
	if isDockerHostEnv() == true {
		return true
	}
	return isDockerSocketPresent()
}
