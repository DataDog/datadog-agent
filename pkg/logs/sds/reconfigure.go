// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive
package sds

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

type ReconfigureOrderType string

const WaitForConfigField = "logs_config.sds.wait_for_configuration"
const WaitForConfigBufferMaxSizeField = "logs_config.sds.buffer_max_size"
const WaitForConfigDefaultBufferMaxSize = 10000

const (
	// StandardRules triggers the storage of a new set of standard rules
	// and reconfigure the internal SDS scanner with an existing user
	// configuration if any.
	StandardRules ReconfigureOrderType = "standard_rules"
	// AgentConfig triggers a reconfiguration of the SDS scanner.
	AgentConfig ReconfigureOrderType = "agent_config"
	// StopProcessing triggers a reconfiguration of the SDS scanner by destroying
	// it to remove the SDS processing step.
	StopProcessing ReconfigureOrderType = "stop_processing"
)

// ReconfigureOrder are used to trigger a reconfiguration
// of the SDS scanner.
type ReconfigureOrder struct {
	Type         ReconfigureOrderType
	Config       []byte
	ResponseChan chan ReconfigureResponse
}

// ReconfigureResponse is used to transmit the result from reconfiguring
// the processors.
type ReconfigureResponse struct {
	Err      error
	IsActive bool
}

// ShouldBlockCollectionUntilSDSConfiguration returns true if we want to start the
// collection only after having received an SDS configuration.
func ShouldBlockCollectionUntilSDSConfiguration(cfg pkgconfigmodel.Reader) bool {
	return cfg.GetString(WaitForConfigField) == "no_collection"
}

// ShouldBufferUntilSDSConfiguration returns true if we have to buffer until we've
// received an SDS configuration.
func ShouldBufferUntilSDSConfiguration(cfg pkgconfigmodel.Reader) bool {
	return cfg.GetString(WaitForConfigField) == "buffer"
}

// WaitForConfigurationBufferMaxSize returns a size for the buffer used while
// waiting for an SDS configuration.
func WaitForConfigurationBufferMaxSize(cfg pkgconfigmodel.Reader) int {
	v := cfg.GetInt(WaitForConfigBufferMaxSizeField)
	if v <= 0 {
		v = WaitForConfigDefaultBufferMaxSize
	}
	return v
}
