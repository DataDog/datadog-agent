// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build (linux_bpf || (windows && npm)) && test

package usm

import (
	"github.com/DataDog/datadog-agent/pkg/network/config"
)

// NewUSMEmptyConfig creates a new network config, with every USM protocols disabled.
func NewUSMEmptyConfig() *config.Config {
	cfg := config.New()
	cfg.ServiceMonitoringEnabled = true
	cfg.EnableHTTPMonitoring = false
	cfg.EnableHTTP2Monitoring = false
	cfg.EnableKafkaMonitoring = false
	cfg.EnablePostgresMonitoring = false
	cfg.EnableRedisMonitoring = false
	cfg.EnableNativeTLSMonitoring = false
	cfg.EnableIstioMonitoring = false
	cfg.EnableNodeJSMonitoring = false
	cfg.EnableGoTLSSupport = false
	cfg.HTTPUseDirectConsumer = false

	return cfg
}
