// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetDeprecatedAttributes(t *testing.T) {
	var attributes []DeprecatedAttribute

	attributes = GetDeprecatedAttributesInUse()
	assert.Equal(t, 0, len(attributes))

	LogsAgent.Set("log_enabled", false)

	attributes = GetDeprecatedAttributesInUse()
	assert.Equal(t, "log_enabled", attributes[0].Name)
	assert.Equal(t, "logs_enabled", attributes[0].Replacement)
}
