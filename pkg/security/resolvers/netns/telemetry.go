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

// attachModeTelemetry is shipped by the runtime-security agent telemetry profile, with the platform tags of the eBPF one
var attachModeTelemetry = metrics.NewITGauge(metrics.MetricTCAttachModeTelemetry,
	[]string{"attach_mode", "platform", "platform_version", "kernel", "arch"},
	"Set to 1 for the attach mode of the TC classifiers")

// withPlatformTags appends the values of the platform tags to the provided tag values
func withPlatformTags(values ...string) []string {
	platform, _ := kernel.Platform()
	platformVersion, _ := kernel.PlatformVersion()
	release, _ := kernel.Release()
	arch, _ := kernel.Machine()
	return append(values, platform, platformVersion, release, arch)
}

// ReportTCAttachMode reports how the TC classifiers are attached to the agent telemetry
func (nr *Resolver) ReportTCAttachMode() {
	if !nr.config.NetworkEnabled {
		return
	}

	mode := nr.tcResolver.AttachMode()
	for _, m := range []manager.TCAttachMode{manager.TCAttachModeLegacy, manager.TCAttachModeTCX} {
		var value float64
		if m == mode {
			value = 1
		}
		attachModeTelemetry.Set(value, withPlatformTags(m.String())...)
	}
}
