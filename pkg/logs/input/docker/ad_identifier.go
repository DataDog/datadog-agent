// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker,!kubelet

package docker

// ContainsADIdentifier returns true if the container contains an autodiscovery identifier.
func ContainsADIdentifier(c *Container) bool {
	_, exists := c.container.Labels[configPath]
	return exists
}
