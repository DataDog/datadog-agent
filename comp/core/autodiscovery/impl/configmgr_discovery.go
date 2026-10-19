// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

import (
	"errors"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/configresolver"
	discovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/discoverer/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// discoveryState holds the fields used when a discovery factory is provided.
type discoveryState struct {
	// discoveryWorker is the workqueue-backed driver that probes integrations
	// to fill in instance configs for Discovery templates.
	discoveryWorker discovery.Worker

	// discoveredCh carries ConfigChanges produced by the discoveryWorker
	// back to AutoConfig.
	discoveredCh chan integration.ConfigChanges
}

// discoveredChangesBuffer is the buffer size for the channel that delivers
// asynchronously-discovered configs to AutoConfig. Sized to absorb a burst
// of completions without blocking the worker goroutine on a busy scheduler.
const discoveredChangesBuffer = 128

// initDiscoveryWorker wires the workqueue-backed discovery worker into cm.
func initDiscoveryWorker(cm *reconcilingConfigManager, factory discovery.Factory) {
	if factory == nil {
		return
	}
	cm.discoveredCh = make(chan integration.ConfigChanges, discoveredChangesBuffer)
	cm.discoveryWorker = factory.NewWorker(cmServiceLookup{cm}, cm.onDiscoveryResult, cm.telemetryStore)
}

func (cm *reconcilingConfigManager) scheduleDiscovery(svcID, tplDigest, integrationName string) {
	if cm.discoveryWorker == nil {
		return
	}
	cm.discoveryWorker.Enqueue(svcID, tplDigest, integrationName)
}

func (cm *reconcilingConfigManager) start() {
	if cm.discoveryWorker == nil {
		return
	}
	cm.discoveryWorker.Start()
}

func (cm *reconcilingConfigManager) stop() {
	if cm.discoveryWorker == nil {
		return
	}
	cm.discoveryWorker.Stop()
}

func (cm *reconcilingConfigManager) discoveredChanges() <-chan integration.ConfigChanges {
	return cm.discoveredCh
}

// cmServiceLookup adapts *reconcilingConfigManager to the
// discovery.ServiceLookup interface without exposing the rest of the manager
// to the discovery engine.
type cmServiceLookup struct {
	cm *reconcilingConfigManager
}

// LookupService implements discovery.ServiceLookup.
func (l cmServiceLookup) LookupService(svcID string) (discovery.ServiceInfo, bool) {
	l.cm.m.Lock()
	defer l.cm.m.Unlock()
	svcAndADIDs, ok := l.cm.activeServices[svcID]
	if !ok {
		return nil, false
	}
	return svcAndADIDs.svc, true
}

// onDiscoveryResult is the callback the discovery worker invokes when a probe
// returns a usable config. It runs in the worker goroutine.
func (cm *reconcilingConfigManager) onDiscoveryResult(svcID, tplDigest string, configs []integration.Config) {
	cm.m.Lock()
	changes := cm.applyDiscoveredConfigsLocked(svcID, tplDigest, configs)
	cm.m.Unlock()
	if len(changes.Schedule) == 0 && len(changes.Unschedule) == 0 {
		return
	}
	select {
	case cm.discoveredCh <- changes:
	default:
		log.Warnf("dropping discovered changes for service %s (channel full)", svcID)
	}
}

// applyDiscoveredConfigsLocked merges a discovered config into a copy of the
// original template, resolves it through the standard configresolver and
// secret-decryption path, and updates the manager's resolution + scheduled
// maps. Returns the ConfigChanges to be applied via the scheduler.
//
// Only the first entry in configs is used today (mirroring the original
// design); integrations that need multiple instances should return a single
// discoveredConfig with multiple instances.
func (cm *reconcilingConfigManager) applyDiscoveredConfigsLocked(svcID, tplDigest string, configs []integration.Config) integration.ConfigChanges {
	var changes integration.ConfigChanges

	svcAndADIDs, ok := cm.activeServices[svcID]
	if !ok {
		// Service went away while the probe was in flight.
		return changes
	}
	tpl, ok := cm.activeConfigs[tplDigest]
	if !ok {
		// Template was removed while the probe was in flight.
		return changes
	}

	// The probe was enqueued (in resolveTemplateForService) when this
	// template was still expected for the service. A probe can take several
	// retry cycles to complete, and by the time it does, a sibling config, a
	// static config, or a generic-integration (openmetrics/prometheus)
	// config may have appeared that would now cause FilterTemplates to drop
	// this template. Re-run that same filtering here, immediately before
	// applying the result, so a slow-arriving probe can't schedule a
	// duplicate/conflicting check on top of a config that showed up while it
	// was in flight.
	if _, stillExpected := cm.expectedFilteredTemplatesLocked(svcID)[tplDigest]; !stillExpected {
		log.Debugf("autodiscovery: discarding stale discovery result for %s on service %s: no longer expected after re-filtering", tpl.Name, svcID)
		return changes
	}

	if len(configs) == 0 || cm.discoveryWorker == nil {
		return changes
	}
	resolved, err := cm.discoveryWorker.ResolveConfig(tpl, configs[0], svcAndADIDs.svc)
	if err != nil {
		if errors.Is(err, configresolver.ErrServiceNotReady) {
			log.Debugf("autodiscovery: discovered config %s for service %s not resolved yet, service not ready", tpl.Name, svcID)
			return changes
		}
		log.Errorf("error resolving discovered config %s for service %s: %v", tpl.Name, svcID, err)
		errorStats.setResolveWarning(tpl.Name, err.Error())
		return changes
	}
	decrypted, err := decryptConfig(resolved, cm.secretResolver, tplDigest)
	if err != nil {
		log.Errorf("error decrypting discovered config %s for service %s: %v", resolved.Name, svcID, err)
		errorStats.setResolveWarning(tpl.Name, err.Error())
		return changes
	}

	existing, ok := cm.serviceResolutions[svcID]
	if !ok {
		existing = map[string]string{}
	}
	if prevDigest, hadPrev := existing[tplDigest]; hadPrev {
		if old, found := cm.scheduledConfigs[prevDigest]; found {
			changes.UnscheduleConfig(old)
		}
	}
	existing[tplDigest] = decrypted.Digest()
	cm.serviceResolutions[svcID] = existing

	changes.ScheduleConfig(decrypted)
	errorStats.removeResolveWarnings(tpl.Name)
	return cm.applyChanges(changes)
}
