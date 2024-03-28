// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package runtimesettingsimpl provide implementation for the runtimesettings.Component
package runtimesettingsimpl

import (
	"testing"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/agent/runtimesettings"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	dogstatsddebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug/serverdebugimpl"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestRuntimeSettings(t *testing.T) {
	var _ = fxutil.Test[runtimesettings.Component](t, fx.Options(
		Module(), // use the real version of this component
		logimpl.MockModule(),
		dogstatsddebug.MockModule(),
	))

	expectedSettings := []string{
		"log_level",
		"runtime_mutex_profile_fraction",
		"runtime_block_profile_rate",
		"dogstatsd_stats",
		"dogstatsd_capture_duration",
		"log_payloads",
		"internal_profiling_goroutines",
		"ha.enabled",
		"ha.failover",
		"internal_profiling",
	}

	for _, name := range expectedSettings {
		_, err := settings.GetRuntimeSetting(name)
		if err != nil {
			t.Errorf("Runtime Setting missing: %s", name)
		}
	}
}
