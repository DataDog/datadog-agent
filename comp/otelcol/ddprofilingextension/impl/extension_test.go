// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddprofilingextension defines the OpenTelemetry Extension implementation.
package ddprofilingextensionimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewExtension(t *testing.T) {
	ext, err := NewExtension(&Config{})
	assert.NoError(t, err)

	_, ok := ext.(*ddExtension)
	assert.True(t, ok)
}
