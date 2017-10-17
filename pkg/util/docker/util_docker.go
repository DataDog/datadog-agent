// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package docker

import (
	"fmt"

	"github.com/docker/docker/api/types/filters"
)

// buildDockerFilter creates a filter.Args object from an even
// number of strings, used as key, value pairs
// An empty "catch-all" filter can be created by passing no argument
func buildDockerFilter(args ...string) (filters.Args, error) {
	filter := filters.NewArgs()
	if len(args)%2 != 0 {
		return filter, fmt.Errorf("an even number of arguments is required")
	}
	for i := 0; i < len(args); i += 2 {
		filter.Add(args[i], args[i+1])
	}
	return filter, nil
}
