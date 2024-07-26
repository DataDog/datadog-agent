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

const waitForConfigField = "logs_config.sds.wait_for_configuration"
const waitForConfigBufferMaxSizeField = "logs_config.sds.buffer_max_size"
const waitForConfigDefaultBufferMaxSize = 1024 * 1024 * 500

const waitForConfigNoCollection = "no_collection"
const waitForConfigBuffer = "buffer"

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

// ValidateConfigField returns true if the configuration value for
// wait_for_configuration is valid.
func ValidateConfigField(cfg pkgconfigmodel.Reader) bool {
	str := cfg.GetString(waitForConfigField)
	return str == "" || str == waitForConfigBuffer || str == waitForConfigNoCollection
}

// ShouldBlockCollectionUntilSDSConfiguration returns true if we want to start the
// collection only after having received an SDS configuration.
func ShouldBlockCollectionUntilSDSConfiguration(cfg pkgconfigmodel.Reader) bool {
	if cfg == nil {
		return false
	}

	return SDSEnabled && cfg.GetString(waitForConfigField) == waitForConfigNoCollection
}

// ShouldBufferUntilSDSConfiguration returns true if we have to buffer until we've
// received an SDS configuration.
func ShouldBufferUntilSDSConfiguration(cfg pkgconfigmodel.Reader) bool {
	if cfg == nil {
		return false
	}

	return SDSEnabled && cfg.GetString(waitForConfigField) == waitForConfigBuffer
}

// WaitForConfigurationBufferMaxSize returns a size for the buffer used while
// waiting for an SDS configuration.
func WaitForConfigurationBufferMaxSize(cfg pkgconfigmodel.Reader) int {
	if cfg == nil {
		return waitForConfigDefaultBufferMaxSize
	}

	v := cfg.GetInt(waitForConfigBufferMaxSizeField)
	if v <= 0 {
		v = waitForConfigDefaultBufferMaxSize
	}
	return v
}
