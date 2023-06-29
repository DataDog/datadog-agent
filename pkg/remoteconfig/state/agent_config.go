// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package state

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/pkg/errors"
)

const agentConfigOrderID = "configuration_order"

var datadogConfigIDRegexp = regexp.MustCompile(`^datadog/\d+/AGENT_CONFIG/([^/]+)/[^/]+$`)

// AgentConfig is a deserialized agent configuration file
// along with the associated metadata
type AgentConfig struct {
	Config   agentConfigData
	Metadata Metadata
}

// ConfigContent contains the configurations set by remote-config
type ConfigContent struct {
	LogLevel string `json:"log_level"`
}

type agentConfigData struct {
	Name   string        `json:"name"`
	Config ConfigContent `json:"config"`
}

// AgentConfigOrder is a deserialized agent configuration file
// along with the associated metadata
type AgentConfigOrder struct {
	Config   agentConfigOrderData
	Metadata Metadata
}

type agentConfigOrderData struct {
	Order         []string `json:"order"`
	InternalOrder []string `json:"internal_order"`
}

// AgentConfigState contains the state of the config in case of fallback or override
type AgentConfigState struct {
	FallbackLogLevel string
	LatestLogLevel   string
}

// parseConfigAgentConfig parses an agent task config
func parseConfigAgentConfig(data []byte, metadata Metadata) (AgentConfig, error) {
	var d agentConfigData

	err := json.Unmarshal(data, &d)
	if err != nil {
		return AgentConfig{}, fmt.Errorf("Unexpected AGENT_CONFIG received through remote-config: %s", err)
	}

	return AgentConfig{
		Config:   d,
		Metadata: metadata,
	}, nil
}

// parseConfigAgentConfig parses an agent task config
func parseConfigAgentConfigOrder(data []byte, metadata Metadata) (AgentConfigOrder, error) {
	var d agentConfigOrderData

	err := json.Unmarshal(data, &d)
	if err != nil {
		return AgentConfigOrder{}, fmt.Errorf("Unexpected AGENT_CONFIG received through remote-config: %s", err)
	}

	return AgentConfigOrder{
		Config:   d,
		Metadata: metadata,
	}, nil
}

// MergeRCAgentConfig is the callback function called when there is an AGENT_CONFIG config update
// The RCClient can directly call back listeners, because there would be no way to send back
// RCTE2 configuration applied state to RC backend.
func MergeRCAgentConfig(applyStatus func(cfgPath string, status ApplyStatus), updates map[string]RawConfig) (ConfigContent, error) {
	var orderFile AgentConfigOrder
	var hasError bool
	var fullErr error
	parsedLayers := map[string]AgentConfig{}

	for configPath, c := range updates {
		var err error
		matched := datadogConfigIDRegexp.FindStringSubmatch(configPath)
		if len(matched) != 2 {
			err = fmt.Errorf("config file path '%s' has wrong format", configPath)
			hasError = true
			fullErr = errors.Wrap(fullErr, err.Error())
			applyStatus(configPath, ApplyStatus{
				State: ApplyStateError,
				Error: err.Error(),
			})
			// If a layer is wrong, fail later to parse the rest and check them all
			continue
		}

		parsedConfigID := matched[1]

		// Ignore the configuration order file
		if parsedConfigID == agentConfigOrderID {
			orderFile, err = parseConfigAgentConfigOrder(c.Config, c.Metadata)
			if err != nil {
				hasError = true
				fullErr = errors.Wrap(fullErr, err.Error())
				applyStatus(configPath, ApplyStatus{
					State: ApplyStateError,
					Error: err.Error(),
				})
				// If a layer is wrong, fail later to parse the rest and check them all
				continue
			}
		} else {
			cfg, err := parseConfigAgentConfig(c.Config, c.Metadata)
			if err != nil {
				hasError = true
				applyStatus(configPath, ApplyStatus{
					State: ApplyStateError,
					Error: err.Error(),
				})
				// If a layer is wrong, fail later to parse the rest and check them all
				continue
			}
			parsedLayers[parsedConfigID] = cfg
		}
	}

	// If there was at least one error, don't apply any config
	if hasError || (len(orderFile.Config.Order) == 0 && len(orderFile.Config.InternalOrder) == 0) {
		return ConfigContent{}, fullErr
	}

	// Go through all the layers that were sent, and apply them one by one to the merged structure
	mergedConfig := ConfigContent{}
	for i := len(orderFile.Config.Order) - 1; i >= 0; i-- {
		if layer, found := parsedLayers[orderFile.Config.Order[i]]; found {
			mergedConfig.LogLevel = layer.Config.Config.LogLevel
		}
	}
	// Same for internal config
	for i := len(orderFile.Config.InternalOrder) - 1; i >= 0; i-- {
		if layer, found := parsedLayers[orderFile.Config.InternalOrder[i]]; found {
			mergedConfig.LogLevel = layer.Config.Config.LogLevel
		}
	}

	return mergedConfig, nil
}
