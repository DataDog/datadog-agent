// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package marshal

import (
	model "github.com/DataDog/agent-payload/v5/process"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// NewAgentConfiguration returns a populated AgentConfiguration ready for the
// wire. Discovery mode force-enables service_monitoring_config.enabled
// internally so newUSMMonitor starts; the wire flag must reflect billing
// intent, so usmEnabled is masked to false when discovery is on.
func NewAgentConfiguration(syscfg, ddcfg pkgconfigmodel.Reader) *model.AgentConfiguration {
	discoveryEnabled := syscfg.GetBool("discovery.service_map.enabled")
	return &model.AgentConfiguration{
		NpmEnabled: syscfg.GetBool("network_config.enabled"),
		// When both USM and discovery are enabled in config, USM still bills:
		// adjustDiscovery flips discovery.service_map.enabled to false before
		// the encoder runs (coexistence rule: USM wins).
		UsmEnabled:                 syscfg.GetBool("service_monitoring_config.enabled") && !discoveryEnabled,
		CcmEnabled:                 syscfg.GetBool("ccm_network_config.enabled"),
		CsmEnabled:                 syscfg.GetBool("runtime_security_config.enabled"),
		EudmEnabled:                ddcfg.GetString("infrastructure_mode") == "end_user_device",
		DiscoveryServiceMapEnabled: discoveryEnabled,
	}
}
