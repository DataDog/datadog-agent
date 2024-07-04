// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package config provides helpers for USM configuration
package config

import (
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
)

// NeedUSM returns true if USM is supported and enabled
func NeedUSM(config *config.Config) bool {
	// http.Supported is misleading, it should be named usm.Supported.
	return config.ServiceMonitoringEnabled && http.Supported()
}

// NeedProcessMonitor returns true if the process monitor is needed for the given configuration
func NeedProcessMonitor(config *config.Config) bool {
	return config.EnableNativeTLSMonitoring || config.EnableGoTLSSupport || config.EnableJavaTLSSupport || config.EnableIstioMonitoring || config.EnableNodeJSMonitoring
}
