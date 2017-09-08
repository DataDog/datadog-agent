// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package collectors

import (
	"fmt"

	"github.com/docker/docker/api/types"
)

// ExtractFromInspect extract tags for a container inspect JSON
func (c *DockerCollector) ExtractFromInspect(co types.ContainerJSON) ([]string, []string, error) {
	var low, high []string
	// TODO: to be completed once docker utils are merged
	low = append(low, "low:test")
	high = append(high, fmt.Sprintf("container_name:%s", co.ID))
	return low, high, nil
}
