// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !docker

package docker

// HostnameProvider docker implementation for the hostname provider
func HostnameProvider() (string, error) {
	return "", ErrDockerNotCompiled
}

// GetTags returns tags that are automatically added to metrics and events on a
// host that is running docker.
func GetTags() ([]string, error) {
	return []string{}, nil
}
