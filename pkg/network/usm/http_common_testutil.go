// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build (linux_bpf || (windows && npm)) && test

package usm

import (
	"github.com/DataDog/datadog-agent/pkg/network/config"
)

// getHTTPCfg creates a new network config with HTTP monitoring enabled and all other USM protocols disabled.
func getHTTPCfg() *config.Config {
	cfg := NewUSMEmptyConfig()
	cfg.EnableHTTPMonitoring = true

	return cfg
}
