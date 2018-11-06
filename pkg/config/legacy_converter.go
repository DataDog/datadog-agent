// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import "strings"

// LegacyConfigConverter should only be used in tests
type LegacyConfigConverter struct {
	Config
}

// Set is used for setting configuration in tests
func (c *LegacyConfigConverter) Set(key string, value interface{}) {
	c.Config.Set(key, value)
}

// NewConfigConverter is creating and returning a mock config
func NewConfigConverter() *LegacyConfigConverter {
	// Configure Datadog global configuration
	Datadog = NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	// Configuration defaults
	initConfig(Datadog)
	return &LegacyConfigConverter{Datadog}
}
