// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

// LegacyConfigConverter is used in the legacy package
// to convert A5 config to A6
type LegacyConfigConverter struct {
	Config
}

// Set is used for setting configuration from A5 config
func (c *LegacyConfigConverter) Set(key string, value interface{}) {
	panic("not called")
}

// NewConfigConverter is creating and returning a config converter
func NewConfigConverter() *LegacyConfigConverter {
	panic("not called")
}
