// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package remoteconfighandler

import (
	"encoding/json"
	"fmt"
)

// multiRegionFailoverConfig is a deserialized Multi-Region Failover configuration file
// Note: This type only deserializes failover_apm, but failover_metrics and failover_logs
// mat also be present on the payload. See the core agent's implementation for reference:
// comp/remote-config/rcclient/rcclientimpl/agent_failover.go
type multiRegionFailoverConfig struct {
	FailoverAPM *bool `json:"failover_apm"`
}

// parseMultiRegionFailoverConfig parses an AGENT_FAILOVER Multi-Region Failover configuration file
func parseMultiRegionFailoverConfig(data []byte) (*multiRegionFailoverConfig, error) {
	var d multiRegionFailoverConfig

	err := json.Unmarshal(data, &d)
	if err != nil {
		return nil, fmt.Errorf("unexpected Multi-Region Failover configs received through remote-config: %s", err)
	}

	return &d, nil
}
