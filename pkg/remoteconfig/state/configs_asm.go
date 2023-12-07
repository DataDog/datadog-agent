// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package state

import (
	"encoding/json"
)

// ConfigASMDD is a deserialized ASM DD configuration file along with its
// associated remote config metadata
type ConfigASMDD struct {
	Config   []byte
	Metadata Metadata
}

func parseConfigASMDD(data []byte, metadata Metadata) (ConfigASMDD, error) {
	return ConfigASMDD{
		Config:   data,
		Metadata: metadata,
	}, nil
}

// ASMDDConfigs returns the currently active ASMDD configs
func (r *Repository) ASMDDConfigs() map[string]ConfigASMDD {
	typedConfigs := make(map[string]ConfigASMDD)

	configs := r.getConfigs(ProductASMDD)

	for path, conf := range configs {
		// We control this, so if this has gone wrong something has gone horribly wrong
		typed, ok := conf.(ConfigASMDD)
		if !ok {
			panic("unexpected config stored as ASMDD Config")
		}

		typedConfigs[path] = typed
	}

	return typedConfigs
}

// ASMFeaturesConfig is a deserialized configuration file that indicates whether ASM should be enabled
// within a tracer, along with its associated remote config metadata.
type ASMFeaturesConfig struct {
	Config   ASMFeaturesData
	Metadata Metadata
}

// ASMFeaturesData describes the state of ASM and some of its features
type ASMFeaturesData struct {
	ASM struct {
		Enabled bool `json:"enabled"`
	} `json:"asm"`
	APISecurity struct {
		RequestSampleRate float64 `json:"request_sample_rate"`
	} `json:"api_security"`
}

func parseASMFeaturesConfig(data []byte, metadata Metadata) (ASMFeaturesConfig, error) {
	var f ASMFeaturesData

	err := json.Unmarshal(data, &f)
	if err != nil {
		return ASMFeaturesConfig{}, nil
	}

	return ASMFeaturesConfig{
		Config:   f,
		Metadata: metadata,
	}, nil
}

// ASMFeaturesConfigs returns the currently active ASMFeatures configs
func (r *Repository) ASMFeaturesConfigs() map[string]ASMFeaturesConfig {
	typedConfigs := make(map[string]ASMFeaturesConfig)

	configs := r.getConfigs(ProductASMFeatures)

	for path, conf := range configs {
		// We control this, so if this has gone wrong something has gone horribly wrong
		typed, ok := conf.(ASMFeaturesConfig)
		if !ok {
			panic("unexpected config stored as ASMFeaturesConfig")
		}

		typedConfigs[path] = typed
	}

	return typedConfigs
}

// ApplyState represents the status of a configuration application by a remote configuration client
// Clients need to either ack the correct application of received configurations, or communicate that
// they haven't applied it yet, or communicate any error that may have happened while doing so
type ApplyState uint64

const (
	//ApplyStateUnknown indicates that a client does not support the ApplyState feature
	ApplyStateUnknown ApplyState = iota
	// ApplyStateUnacknowledged indicates a client has received the config but has not specified success or failure
	ApplyStateUnacknowledged
	// ApplyStateAcknowledged indicates a client has successfully applied the config
	ApplyStateAcknowledged
	// ApplyStateError indicates that a client has failed to apply the config
	ApplyStateError
)

// ApplyStatus is the processing status for a given configuration.
// It basically represents whether a config was successfully processed and apply, or if an error occurred
type ApplyStatus struct {
	State ApplyState
	Error string
}

// ASMDataConfig is a deserialized configuration file that holds rules data that can be used
// by the ASM WAF for specific features (example: ip blocking).
type ASMDataConfig struct {
	Config   ASMDataRulesData
	Metadata Metadata
}

// ASMDataRulesData is a serializable array of rules data entries
type ASMDataRulesData struct {
	RulesData []ASMDataRuleData `json:"rules_data"`
}

// ASMDataRuleData is an entry in the rules data list held by an ASMData configuration
type ASMDataRuleData struct {
	ID   string                 `json:"id"`
	Type string                 `json:"type"`
	Data []ASMDataRuleDataEntry `json:"data"`
}

// ASMDataRuleDataEntry represents a data entry in a rule data file
type ASMDataRuleDataEntry struct {
	Expiration int64  `json:"expiration,omitempty"`
	Value      string `json:"value"`
}

func parseConfigASMData(data []byte, metadata Metadata) (ASMDataConfig, error) {
	cfg := ASMDataConfig{
		Metadata: metadata,
	}
	err := json.Unmarshal(data, &cfg.Config)
	return cfg, err
}

// ASMDataConfigs returns the currently active ASMData configs
func (r *Repository) ASMDataConfigs() map[string]ASMDataConfig {
	typedConfigs := make(map[string]ASMDataConfig)
	configs := r.getConfigs(ProductASMData)

	for path, cfg := range configs {
		// We control this, so if this has gone wrong something has gone horribly wrong
		typed, ok := cfg.(ASMDataConfig)
		if !ok {
			panic("unexpected config stored as ASMDataConfig")
		}
		typedConfigs[path] = typed
	}

	return typedConfigs
}
