// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package common

import (
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/apminject"
)

func apmInjectorRequiresReinstall(mode string) bool {
	if apminject.RequiresReinstallForIncompatibleVersion() {
		return true
	}
	switch mode {
	case env.APMInstrumentationEnabledDocker:
		return apminject.DockerRuntimeNeedsRepair()
	case env.APMInstrumentationEnabledHost:
		return apminject.HostPreloadNeedsRepair()
	case env.APMInstrumentationEnabledAll, env.APMInstrumentationNotSet:
		return apminject.AllInstrumentationNeedsRepair()
	default:
		return false
	}
}
