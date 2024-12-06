// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
package npcollectorimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNetworkPathCollectorEnabled(t *testing.T) {
	config := &collectorConfigs{
		connectionsMonitoringEnabled: true,
	}
	assert.True(t, config.networkPathCollectorEnabled())

	config.connectionsMonitoringEnabled = false
	assert.False(t, config.networkPathCollectorEnabled())
}
