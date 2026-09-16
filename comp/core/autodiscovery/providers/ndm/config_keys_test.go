// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndm

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestProviderName(t *testing.T) {
	assert.Equal(t, "ndm-remote-config", names.NDMRemoteConfig)
}

func TestRemoteConfigKeysAreDeclaredAndDefaultOff(t *testing.T) {
	cfg := configmock.New(t)

	assert.False(t, cfg.GetBool("network_devices.remote_config.enabled"))
	assert.Empty(t, cfg.GetStringSlice("network_devices.snmp_credentials"))
	assert.True(t, cfg.IsKnown("network_devices.remote_config.enabled"))
	assert.True(t, cfg.IsKnown("network_devices.snmp_credentials"))
}
