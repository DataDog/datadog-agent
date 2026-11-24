// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netns

import (
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tc"
)

func TestReportTCAttachMode(t *testing.T) {
	attachMode := func(mode manager.TCAttachMode) float64 {
		return attachModeTelemetry.WithValues(withPlatformTags(mode.String())...).Get()
	}
	report := func(cfg *config.Config) {
		nr := &Resolver{config: cfg, tcResolver: tc.NewResolver(cfg)}
		nr.ReportTCAttachMode()
	}

	for _, test := range []struct {
		name           string
		tcxEnabled     bool
		expectedLegacy float64
		expectedTCX    float64
	}{
		{name: "tcx", tcxEnabled: true, expectedTCX: 1},
		{name: "legacy", expectedLegacy: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			report(&config.Config{NetworkEnabled: true, NetworkTCXEnabled: test.tcxEnabled})

			assert.Equal(t, test.expectedLegacy, attachMode(manager.TCAttachModeLegacy))
			assert.Equal(t, test.expectedTCX, attachMode(manager.TCAttachModeTCX))
		})
	}

	t.Run("network disabled", func(t *testing.T) {
		report(&config.Config{NetworkEnabled: true})
		report(&config.Config{NetworkTCXEnabled: true})

		assert.Equal(t, 1.0, attachMode(manager.TCAttachModeLegacy), "nothing is reported without network capture")
		assert.Zero(t, attachMode(manager.TCAttachModeTCX))
	})
}
