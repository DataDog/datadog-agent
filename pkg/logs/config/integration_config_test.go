// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !windows

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateShouldSucceedWithValidConfigs(t *testing.T) {
	validConfigs := []*LogsConfig{
		{Type: FileType, Path: "/var/log/foo.log"},
		{Type: TCPType, Port: 1234},
		{Type: UDPType, Port: 5678},
		{Type: DockerType},
		{Type: JournaldType, ProcessingRules: []*ProcessingRule{{Name: "foo", Type: ExcludeAtMatch, Pattern: ".*"}}},
	}

	for _, config := range validConfigs {
		err := config.Validate()
		assert.Nil(t, err)
	}
}

func TestValidateShouldFailWithInvalidConfigs(t *testing.T) {
	invalidConfigs := []*LogsConfig{
		{},
		{Type: FileType},
		{Type: TCPType},
		{Type: UDPType},
		{Type: DockerType, ProcessingRules: []*ProcessingRule{{Name: "foo"}}},
		{Type: DockerType, ProcessingRules: []*ProcessingRule{{Name: "foo", Type: "bar"}}},
		{Type: DockerType, ProcessingRules: []*ProcessingRule{{Name: "foo", Type: ExcludeAtMatch}}},
		{Type: DockerType, ProcessingRules: []*ProcessingRule{{Name: "foo", Pattern: ".*"}}},
		{Type: DockerType, ProcessingRules: []*ProcessingRule{{Type: ExcludeAtMatch, Pattern: ".*"}}},
		{Type: DockerType, ProcessingRules: []*ProcessingRule{{Type: ExcludeAtMatch}}},
		{Type: DockerType, ProcessingRules: []*ProcessingRule{{Pattern: ".*"}}},
	}

	for _, config := range invalidConfigs {
		err := config.Validate()
		assert.NotNil(t, err)
	}
}
