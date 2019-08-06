// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !python

package status

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector"
)

// GetPythonCheckMemoryUsage gets the memory usage for all checks as JSON
func GetPythonCheckMemoryUsage(c *collector.Collector) ([]byte, error) {
	return nil, fmt.Errorf("Python support unavailable on build")
}
