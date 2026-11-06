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
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tc"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestReportTCAttachMode(t *testing.T) {
	attachMode := func(mode manager.TCAttachMode) float64 {
		return attachModeTelemetry.WithValues(withPlatformTags(mode.String())...).Get()
	}
	tcxUnsupported := func() float64 {
		return tcxUnsupportedTelemetry.WithValues(withPlatformTags()...).Get()
	}
	report := func(cfg *config.Config, tcxUnsupported bool) {
		nr := &Resolver{config: cfg, tcResolver: tc.NewResolver(cfg)}
		nr.ReportTCAttachMode(tcxUnsupported)
	}

	for _, test := range []struct {
		name                string
		tcxEnabled          bool
		tcxUnsupported      bool
		expectedLegacy      float64
		expectedTCX         float64
		expectedUnsupported float64
	}{
		{name: "tcx", tcxEnabled: true, expectedTCX: 1},
		{name: "tcx unsupported", tcxUnsupported: true, expectedLegacy: 1, expectedUnsupported: 1},
		{name: "legacy", expectedLegacy: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			report(&config.Config{NetworkEnabled: true, NetworkTCXEnabled: test.tcxEnabled}, test.tcxUnsupported)

			assert.Equal(t, test.expectedLegacy, attachMode(manager.TCAttachModeLegacy))
			assert.Equal(t, test.expectedTCX, attachMode(manager.TCAttachModeTCX))
			assert.Equal(t, test.expectedUnsupported, tcxUnsupported())
		})
	}

	t.Run("network disabled", func(t *testing.T) {
		report(&config.Config{NetworkEnabled: true}, false)
		report(&config.Config{NetworkTCXEnabled: true}, true)

		assert.Equal(t, 1.0, attachMode(manager.TCAttachModeLegacy), "nothing is reported without network capture")
		assert.Zero(t, attachMode(manager.TCAttachModeTCX))
		assert.Zero(t, tcxUnsupported())
	})
}

func TestTCXAttachErrorsTelemetry(t *testing.T) {
	count := func(class string) float64 {
		return tcxAttachErrorsTelemetry.WithValues(withPlatformTags(class)...).Get()
	}
	setnsBefore, tcxAttachBefore := count(errorClassSetns), count(errorClassTCXAttach)

	nr := &Resolver{errorCounters: newErrorCounters()}
	device := model.NetDevice{Name: "eth0", IfIndex: 2, NetNS: 4026532420}
	for _, err := range []error{
		attachErr(tcxAttachErr(unix.ERANGE)),
		attachErr(tcxAttachErr(unix.EPERM)),
		attachErr(setnsErr(unix.EPERM)),
		attachErr(tcxAttachErr(unix.ENODEV)),
		linkNotFoundErr(t),
	} {
		nr.reportTCClassifierError(err, device)
	}

	assert.Equal(t, tcxAttachBefore+2, count(errorClassTCXAttach))
	assert.Equal(t, setnsBefore+1, count(errorClassSetns))
	assert.Zero(t, count(errorClassNoSuchDevice), "an interface that vanishes isn't a TCX attach error")
	assert.Zero(t, count(errorClassLinkNotFound))
}

func TestStaleLinksTelemetry(t *testing.T) {
	count := func() float64 {
		return staleLinksTelemetry.WithValues(withPlatformTags()...).Get()
	}
	before := count()

	countStaleLinks(0)
	assert.Equal(t, before, count())

	countStaleLinks(2)
	countStaleLinks(1)
	assert.Equal(t, before+3, count())
}
