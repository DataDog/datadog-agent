// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netns

import (
	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// platformTags are the tags of the eBPF agent telemetry profile, which the network classifiers metrics share
var platformTags = []string{"platform", "platform_version", "kernel", "arch"}

// agent telemetry metrics, shipped by the runtime-security profile
var (
	blindHooksTelemetry = metrics.NewITGauge(metrics.MetricTCBlindHooksTelemetry,
		append([]string{"attach_mode", "reason", "direction"}, platformTags...),
		"Number of hooks of host interfaces where a foreign program runs before the TC classifiers")
	attachModeTelemetry = metrics.NewITGauge(metrics.MetricTCAttachModeTelemetry,
		append([]string{"attach_mode"}, platformTags...),
		"Set to 1 for the attach mode of the TC classifiers")
	tcxUnsupportedTelemetry = metrics.NewITGauge(metrics.MetricTCXUnsupportedTelemetry,
		platformTags,
		"Set to 1 when TCX is enabled on a kernel that doesn't support it")
	tcxAttachErrorsTelemetry = metrics.NewITCounter(metrics.MetricTCXAttachErrorsTelemetry,
		append([]string{"reason"}, platformTags...),
		"Number of TC classifiers that couldn't be attached with TCX links, by class of error")
	staleLinksTelemetry = metrics.NewITCounter(metrics.MetricTCStaleLinksTelemetry,
		platformTags,
		"Number of TCX links detached because their device left the network namespace in which they were attached")
)

// withPlatformTags appends the values of the platform tags to the provided tag values
func withPlatformTags(values ...string) []string {
	platform, _ := kernel.Platform()
	platformVersion, _ := kernel.PlatformVersion()
	release, _ := kernel.Release()
	arch, _ := kernel.Machine()
	return append(values, platform, platformVersion, release, arch)
}

func gaugeValue(set bool) float64 {
	if set {
		return 1
	}
	return 0
}

// ReportTCAttachMode reports how the TC classifiers are attached to the agent telemetry, along with whether TCX is
// enabled on a kernel that doesn't support it
func (nr *Resolver) ReportTCAttachMode(tcxUnsupported bool) {
	if !nr.config.NetworkEnabled {
		return
	}

	mode := nr.tcResolver.AttachMode()
	for _, m := range []manager.TCAttachMode{manager.TCAttachModeLegacy, manager.TCAttachModeTCX} {
		attachModeTelemetry.Set(gaugeValue(m == mode), withPlatformTags(m.String())...)
	}
	tcxUnsupportedTelemetry.Set(gaugeValue(tcxUnsupported), withPlatformTags()...)
}

func countTCXAttachError(class string) {
	tcxAttachErrorsTelemetry.Inc(withPlatformTags(class)...)
}

func countStaleLinks(count int) {
	if count > 0 {
		staleLinksTelemetry.Add(float64(count), withPlatformTags()...)
	}
}
