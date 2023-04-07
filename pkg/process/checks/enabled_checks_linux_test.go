// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package checks

import (
	"testing"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestProcessEventsCheckEnabled(t *testing.T) {
	scfg := &sysconfig.Config{}

	t.Run("default", func(t *testing.T) {
		config.Mock(t)

		enabledChecks := getEnabledChecks(scfg)
		assertNotContainsCheck(t, enabledChecks, ProcessEventsCheckName)
	})

	t.Run("enabled", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.Set("process_config.event_collection.enabled", true)

		enabledChecks := getEnabledChecks(scfg)
		assertContainsCheck(t, enabledChecks, ProcessEventsCheckName)
	})

	t.Run("disabled", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.Set("process_config.event_collection.enabled", false)

		enabledChecks := getEnabledChecks(scfg)
		assertNotContainsCheck(t, enabledChecks, ProcessEventsCheckName)
	})
}
