// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package ddhostnameprocessor implements an OTel processor that injects
// datadog.host.name into resource attributes for standalone mode.
package ddhostnameprocessor

import "go.opentelemetry.io/collector/component"

type Config struct{}

var _ component.Config = (*Config)(nil)

func (c *Config) Validate() error {
	return nil
}
