// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sbom

import (
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
)

// Config holds the configuration for the runtime security agent
type Config struct {
	// SBOMResolverEnabled defines if the SBOM resolver should be enabled
	SBOMResolverEnabled bool

	// SBOMResolverWorkloadsCacheSize defines the count of SBOMs to keep in memory in order to prevent re-computing
	// the SBOMs of short-lived and periodical workloads
	SBOMResolverWorkloadsCacheSize int
}

// NewConfig returns a new Config object
func NewConfig() *Config {
	return &Config{
		// SBOM resolver
		SBOMResolverEnabled:            coreconfig.SystemProbe.GetBool("runtime_security_config.sbom.enabled"),
		SBOMResolverWorkloadsCacheSize: coreconfig.SystemProbe.GetInt("runtime_security_config.sbom.workloads_cache_size"),
	}
}
