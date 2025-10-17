// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp

// Package configcheck exposes helpers to fetch config.
package configcheck

import (
	"go.opentelemetry.io/collector/confmap"

	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// ReadConfigSection from a config.Component object.
func ReadConfigSection(cfg configmodel.Reader, section string) *confmap.Conf {
	return confmap.NewFromStringMap(readConfigSection(cfg, section))
}

// IsEnabled checks if OTLP pipeline is enabled in a given config, and the binary has OTLP support.
func IsEnabled(cfg configmodel.Reader) bool {
	return IsConfigEnabled(cfg)
}

// IsDisplayed checks if the OTLP section should be rendered in the Agent
func IsDisplayed() bool {
	return true
}
