// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package traceroute

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetPorts(t *testing.T) {
	destPort, sourcePort, useSourcePort := getPorts(0)
	assert.GreaterOrEqual(t, destPort, uint16(DefaultDestPort))
	assert.GreaterOrEqual(t, sourcePort, uint16(DefaultSourcePort))
	assert.False(t, useSourcePort)

	destPort, sourcePort, useSourcePort = getPorts(80)
	assert.Equal(t, destPort, uint16(80))
	assert.GreaterOrEqual(t, sourcePort, uint16(DefaultSourcePort))
	assert.True(t, useSourcePort)
}
