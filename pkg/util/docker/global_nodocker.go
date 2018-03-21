// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !docker

package docker

var (
	// NullContainer is an empty container object that has
	// default values for all fields including sub-fields.
	// If new sub-structs are added to Container this must
	// be updated.
	NullContainer = &Container{
		CPU:     &CgroupTimesStat{},
		Memory:  &CgroupMemStat{},
		IO:      &CgroupIOStat{},
		Network: ContainerNetStats{},
	}
)

// HostnameProvider docker implementation for the hostname provider
func HostnameProvider(hostName string) (string, error) {
	return "", ErrDockerNotCompiled
}

// IsContainerized returns True if we're running in the docker-dd-agent container.
func IsContainerized() bool {
	return false
}
