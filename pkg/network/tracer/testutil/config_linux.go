// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package testutil

import (
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var kv = kernel.MustHostVersion()

// Config returns a network.Config setup for test purposes
func Config() *config.Config {
	cfg := config.New()
	if ddconfig.IsECSFargate() {
		// protocol classification not yet supported on fargate
		cfg.ProtocolClassificationEnabled = false
	}
	// prebuilt on 5.18+ does not support UDPv6
	if isPrebuilt(cfg) && kv >= kernel.VersionCode(5, 18, 0) {
		cfg.CollectUDPv6Conns = false
	}

	cfg.EnableGatewayLookup = false
	return cfg
}

func isPrebuilt(cfg *config.Config) bool {
	if cfg.EnableRuntimeCompiler || cfg.EnableCORE {
		return false
	}
	return true
}
