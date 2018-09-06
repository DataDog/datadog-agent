// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker,!kubelet

package docker

import (
	"github.com/docker/docker/api/types"
)

// configPath refers to the configuration that can be passed over a docker label,
// this feature is commonly named 'ad' or 'autodicovery'.
const configPath = "com.datadoghq.ad.logs"

// ContainsADIdentifier returns true if the container contains an autodiscovery identifier.
func ContainsADIdentifier(c *Container) bool {
	_, exists := container.Labels[configPath]
	return exists
}
