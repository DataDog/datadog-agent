// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hfrunnerimpl

import (
	"sync"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"

	taggerdef "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilterdef "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmetadef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/net/network"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/cpu/cpu"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/cpu/load"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/disk/disk"
	diskio "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/disk/io"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/filehandles"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/memory"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/uptime"
)

const (
	// tickInterval is how often each check is run.
	tickInterval = time.Second

	// initialBackoff is the backoff after the first failure.
	initialBackoff = 2 * time.Second

	// maxBackoff caps the retry wait to avoid long silences.
	maxBackoff = 60 * time.Second
)

// systemCheckSources is the set of MetricSource values produced by the system
// checks that the HF runner executes. It mirrors the check list in newRunner.
var systemCheckSources = map[metrics.MetricSource]struct{}{
	metrics.MetricSourceCPU:        {},
	metrics.MetricSourceLoad:       {},
	metrics.MetricSourceMemory:     {},
	metrics.MetricSourceIo:         {},
	metrics.MetricSourceDisk:       {},
	metrics.MetricSourceNetwork:    {},
	metrics.MetricSourceUptime:     {},
	metrics.MetricSourceFileHandle: {},
}

// containerCheckSources is the set of MetricSource values produced by the
// container checks that the HF container runner executes.
var containerCheckSources = map[metrics.MetricSource]struct{}{
	metrics.MetricSourceContainer: {},
}

// ContainerDeps holds the components required to run the generic container check.
// All three must be non-nil; newContainerRunner returns nil if any are missing.
type ContainerDeps struct {
	WMeta       workloadmetadef.Component
	FilterStore workloadfilterdef.Component
	Tagger      taggerdef.Component
}

// checkEntry tracks a single check instance and its retry state.
type checkEntry struct {
	name    string
	ch      check.Check
	backoff time.Duration // current backoff duration; 0 = no active backoff
	retryAt time.Time     // when to next attempt after a failure
	logged  bool          // whether we've already logged the first failure
}

// runner runs system checks at 1-second intervals and routes their output
// directly into the observer pipeline. It has no interaction with the normal
// collector/aggregator/forwarder chain.
type runner struct {
	entries  []*checkEntry
	stopCh   chan struct{}
	stopOnce sync.Once
}

// newRunner creates a runner, instantiates system checks, and configures them.
// Checks that are unavailable on the current platform are silently skipped.
func newRunner(handle observerdef.Handle) *runner {
	mgr := newObserverSenderManager(handle)

	type factoryEntry struct {
		name    string
		factory option.Option[func() check.Check]
	}

	factories := []factoryEntry{
		{"cpu", cpu.Factory()},
		{"load", load.Factory()},
		{"memory", memory.Factory()},
		{"disk", disk.Factory()},
		{"io", diskio.Factory()},
		{"network", network.Factory()},
		{"uptime", uptime.Factory()},
		{"filehandles", filehandles.Factory()},
	}

	var entries []*checkEntry
	for _, fe := range factories {
		factory, ok := fe.factory.Get()
		if !ok {
			log.Debugf("[observer/hfrunner] %s check not available on this platform, skipping", fe.name)
			continue
		}

		ch := factory()

		err := ch.Configure(mgr, 0, integration.Data("{}"), integration.Data("{}"), "hf-runner", "hf-runner")
		if err != nil {
			log.Warnf("[observer/hfrunner] failed to configure %s check, skipping: %v", fe.name, err)
			continue
		}

		entries = append(entries, &checkEntry{name: fe.name, ch: ch})
	}

	log.Infof("[observer/hfrunner] initialized %d system checks for high-frequency collection", len(entries))
	return &runner{entries: entries, stopCh: make(chan struct{})}
}

// newContainerRunner creates a runner that collects container metrics at 1-second
// intervals via the generic container check. Returns nil if any dep in ContainerDeps is nil.
func newContainerRunner(handle observerdef.Handle, deps ContainerDeps) *runner {
	if deps.WMeta == nil || deps.FilterStore == nil || deps.Tagger == nil {
		log.Warn("[observer/hfrunner] container check deps incomplete, skipping container HF runner")
		return nil
	}

	mgr := newObserverSenderManager(handle)

	type factoryEntry struct {
		name    string
		factory option.Option[func() check.Check]
	}

	factories := []factoryEntry{
		{"container", generic.Factory(deps.WMeta, deps.FilterStore, deps.Tagger)},
	}

	var entries []*checkEntry
	for _, fe := range factories {
		factory, ok := fe.factory.Get()
		if !ok {
			log.Debugf("[observer/hfrunner] %s check not available on this platform, skipping", fe.name)
			continue
		}
		ch := factory()
		err := ch.Configure(mgr, 0, integration.Data("{}"), integration.Data("{}"), "hf-container-runner", "hf-container-runner")
		if err != nil {
			log.Warnf("[observer/hfrunner] failed to configure %s check, skipping: %v", fe.name, err)
			continue
		}
		entries = append(entries, &checkEntry{name: fe.name, ch: ch})
	}

	log.Infof("[observer/hfrunner] initialized %d container checks for high-frequency collection", len(entries))
	return &runner{entries: entries, stopCh: make(chan struct{})}
}

// start begins the 1-second collection loop in a background goroutine.
func (r *runner) start() {
	go r.run()
}

// stop signals the collection loop to exit. Safe to call multiple times.
func (r *runner) stop() {
	r.stopOnce.Do(func() { close(r.stopCh) })
}

func (r *runner) run() {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case now := <-ticker.C:
			for _, e := range r.entries {
				r.runCheck(e, now)
			}
		}
	}
}

func (r *runner) runCheck(e *checkEntry, now time.Time) {
	if e.backoff > 0 && now.Before(e.retryAt) {
		return
	}

	if err := e.ch.Run(); err != nil {
		if !e.logged {
			log.Warnf("[observer/hfrunner] %s check failed (will retry with backoff): %v", e.name, err)
			e.logged = true
		}
		if e.backoff == 0 {
			e.backoff = initialBackoff
		} else {
			e.backoff *= 2
			if e.backoff > maxBackoff {
				e.backoff = maxBackoff
			}
		}
		e.retryAt = now.Add(e.backoff)
		return
	}

	if e.backoff > 0 {
		log.Infof("[observer/hfrunner] %s check recovered", e.name)
	}
	e.backoff = 0
	e.logged = false
}

// copySourceSet returns a copy of the given MetricSource set.
func copySourceSet(src map[metrics.MetricSource]struct{}) map[metrics.MetricSource]struct{} {
	dst := make(map[metrics.MetricSource]struct{}, len(src))
	for k := range src {
		dst[k] = struct{}{}
	}
	return dst
}
