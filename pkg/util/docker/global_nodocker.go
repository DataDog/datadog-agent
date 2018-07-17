// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !docker

package docker

// HostnameProvider docker implementation for the hostname provider
func HostnameProvider(hostName string) (string, error) {
	return "", ErrDockerNotCompiled
}

// IsContainerized returns True if we're running in the docker-dd-agent container.
func IsContainerized() bool {
	return false
}

// GetTags returns tags that are automatically added to metrics and events on a
// host that is running docker.
func GetTags() ([]string, error) {
	return []string{}, nil
}
