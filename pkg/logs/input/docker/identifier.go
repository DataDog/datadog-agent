// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

// shortLength represents the maximum length of a short container identifier.
const shortLength = 12

// ShortContainerID shortens the container identifier.
func ShortContainerID(containerID string) string {
	if len(containerID) <= shortLength {
		return containerID
	}
	return containerID[:shortLength]
}
