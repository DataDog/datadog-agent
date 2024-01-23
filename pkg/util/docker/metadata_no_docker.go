// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !docker

package docker

// GetMetadata returns metadata about the docker runtime such as docker_version and if docker_swarm is enabled or not.
func GetMetadata() (map[string]string, error) {
	panic("not called")
}
