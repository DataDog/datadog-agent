// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package rcclientimpl

import (
	"encoding/json"
	"fmt"
)

// highAvailabilityConfig is a deserialized high availability configuration file
type highAvailabilityConfig struct {
	Failover *bool `json:"failover"`
}

// parseHighAvailabilityConfig parses an agent task config
func parseHighAvailabilityConfig(data []byte) (*highAvailabilityConfig, error) {
	var d highAvailabilityConfig

	err := json.Unmarshal(data, &d)
	if err != nil {
		return nil, fmt.Errorf("unexpected failover configs received through remote-config: %s", err)
	}

	return &d, nil
}
