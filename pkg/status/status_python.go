// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build python

package status

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetPythonCheckMemoryUsage gets the memory usage for all checks as JSON
func GetPythonCheckMemoryUsage(c *collector.Collector) ([]byte, error) {
	instances := c.GetAllCheckInstances()
	stats := make(map[string]map[check.ID]int64)

	for _, chk := range instances {
		pyCheck, ok := chk.(*python.PythonCheck)
		if !ok {
			continue
		}

		cSize, err := pyCheck.SizeOfCheck()
		size := int64(cSize)
		if err != nil {
			log.Errorf("Error collecting size for instance %v: %v", pyCheck.ID(), err)
			continue
		}

		_, ok = stats[pyCheck.String()]
		if !ok {
			stats[pyCheck.String()] = make(map[check.ID]int64)
		}
		stats[pyCheck.String()][pyCheck.ID()] = size
	}

	statsJSON, err := json.Marshal(stats)
	if err != nil {
		return nil, err
	}

	return statsJSON, nil
}
