// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package enablement

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestConnectionDynamicTestsEnabled(t *testing.T) {
	core := mock.New(t)
	sysprobe := mock.NewSystemProbe(t)

	assert.False(t, ConnectionDynamicTestsEnabled(core, sysprobe))

	sysprobe.SetInTest("network_config.enabled", true)
	assert.True(t, ConnectionDynamicTestsEnabled(core, sysprobe))

	core.SetInTest("network_path.connections_monitoring.basic_tests_enabled", false)
	assert.False(t, ConnectionDynamicTestsEnabled(core, sysprobe))

	core.SetInTest("network_path.connections_monitoring.enabled", true)
	assert.True(t, ConnectionDynamicTestsEnabled(core, sysprobe))

	sysprobe.SetInTest("network_config.enabled", false)
	assert.False(t, ConnectionDynamicTestsEnabled(core, sysprobe))
}
