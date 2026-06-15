// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package autodiscoveryimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/discoverer"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

// discoveredChangesBuffer is the buffer size for the channel that delivers
// asynchronously-discovered configs to AutoConfig. Sized to absorb a burst
// of completions without blocking the worker goroutine on a busy scheduler.
const discoveredChangesBuffer = 128

// initDiscoveryWorker wires the workqueue-backed discovery worker into cm.
func initDiscoveryWorker(cm *reconcilingConfigManager, disco discoverer.ConfigDiscoverer) {
	cm.discoveredCh = make(chan integration.ConfigChanges, discoveredChangesBuffer)
	cm.discoveryWorker = discoverer.NewWorker(disco, cmServiceLookup{cm}, cm.onDiscoveryResult, discoverer.Config{})
}

func (cm *reconcilingConfigManager) scheduleDiscovery(svcID, tplDigest, integrationName string) {
	cm.discoveryWorker.Enqueue(svcID, tplDigest, integrationName)
}

func (cm *reconcilingConfigManager) start() {
	cm.discoveryWorker.Start()
}

func (cm *reconcilingConfigManager) stop() {
	cm.discoveryWorker.Stop()
}

func (cm *reconcilingConfigManager) discoveredChanges() <-chan integration.ConfigChanges {
	return cm.discoveredCh
}
