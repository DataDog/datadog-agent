// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !python

package autodiscoveryimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/discoverer"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

// discoveryState is empty in non-python builds; the fields are only needed
// when the discovery worker is active.
type discoveryState struct{} //nolint:unused

// initDiscoveryWorker is a no-op in non-Python builds. Keeping it empty means
// the linker never sees a call to discoverer.NewWorker for dead-code elimination.
func initDiscoveryWorker(_ *reconcilingConfigManager, _ discoverer.ConfigDiscoverer) {}

func (cm *reconcilingConfigManager) scheduleDiscovery(_, _, _ string) {}

func (cm *reconcilingConfigManager) start() {}
func (cm *reconcilingConfigManager) stop()  {}

// discoveredChanges returns nil in non-Python builds.
func (cm *reconcilingConfigManager) discoveredChanges() <-chan integration.ConfigChanges {
	return nil
}
