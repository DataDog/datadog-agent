// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed by Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pathConfig struct {
	values map[string]interface{}
	source Source
}

func (c *pathConfig) Get(key string) interface{} { return c.values[key] }
func (c *pathConfig) IsKnown(key string) bool    { _, found := c.values[key]; return found }
func (c *pathConfig) Set(key string, value interface{}, source Source) {
	c.values[key], c.source = value, source
}

func TestAssignAtPath(t *testing.T) {
	config := &pathConfig{values: map[string]interface{}{
		"additional_endpoints": map[string][]string{
			"https://example.com": {"static", "DELA(org, aws)"},
		},
	}}

	err := AssignAtPath(config, []string{"additional_endpoints", "https://example.com", "1"}, "resolved", SourceSecret)
	require.NoError(t, err)
	assert.Equal(t, []string{"static", "resolved"}, config.values["additional_endpoints"].(map[string][]string)["https://example.com"])
	assert.Equal(t, SourceSecret, config.source)
}

func TestAssignAtPathRejectsInvalidPath(t *testing.T) {
	config := &pathConfig{values: map[string]interface{}{"items": []string{"one"}}}

	assert.Error(t, AssignAtPath(config, []string{"unknown", "0"}, "value", SourceSecret))
	assert.Error(t, AssignAtPath(config, []string{"items", "2"}, "value", SourceSecret))
	assert.Error(t, AssignAtPath(config, []string{"items", "-1"}, "value", SourceSecret))
}
