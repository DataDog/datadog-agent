// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package rcclientimpl

import (
	"encoding/json"
	"fmt"
)

// multiRegionFailoverConfig is a deserialized multi-region failover configuration file
type multiRegionFailoverConfig struct {
	Failover *bool `json:"failover"`
}

// parseMultiRegionFailoverConfig parses an agent task config
func parseMultiRegionFailoverConfig(data []byte) (*multiRegionFailoverConfig, error) {
	var d multiRegionFailoverConfig

	err := json.Unmarshal(data, &d)
	if err != nil {
		return nil, fmt.Errorf("unexpected failover configs received through remote-config: %s", err)
	}

	return &d, nil
}
