// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package hfrunnerimpl

import (
	"sync"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"

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
	tickInterval   = time.Second
	initialBackoff = 2 * time.Second
	maxBackoff     = 60 * time.Second
)

type factoryEntry struct {
	name    string
	factory option.Option[func() check.Check]
}

type checkEntry struct {
	name    string
	ch      check.Check
	backoff time.Duration
	retryAt time.Time
	logged  bool
}

type runner struct {
	entries  []*checkEntry
	stopCh   chan struct{}
	stopOnce sync.Once
}

func (r *runner) start() { go r.run() }

func (r *runner) stop() { r.stopOnce.Do(func() { close(r.stopCh) }) }

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

func newRunner(handle observerdef.Handle) *runner {
	mgr := newObserverSenderManager(handle)
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
		if err := ch.Configure(mgr, 0, integration.Data("{}"), integration.Data("{}"), "hf-runner", "hf-runner"); err != nil {
			log.Warnf("[observer/hfrunner] failed to configure %s check, skipping: %v", fe.name, err)
			continue
		}
		entries = append(entries, &checkEntry{name: fe.name, ch: ch})
	}
	log.Infof("[observer/hfrunner] initialized %d system checks for high-frequency collection", len(entries))
	return &runner{entries: entries, stopCh: make(chan struct{})}
}

func newContainerRunner(handle observerdef.Handle, deps ContainerDeps) *runner {
	if deps.WMeta == nil || deps.FilterStore == nil || deps.Tagger == nil {
		log.Warn("[observer/hfrunner] container check deps incomplete, skipping container HF runner")
		return nil
	}
	mgr := newObserverSenderManager(handle)
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
		if err := ch.Configure(mgr, 0, integration.Data("{}"), integration.Data("{}"), "hf-container-runner", "hf-container-runner"); err != nil {
			log.Warnf("[observer/hfrunner] failed to configure %s check, skipping: %v", fe.name, err)
			continue
		}
		entries = append(entries, &checkEntry{name: fe.name, ch: ch})
	}
	log.Infof("[observer/hfrunner] initialized %d container checks for high-frequency collection", len(entries))
	return &runner{entries: entries, stopCh: make(chan struct{})}
}
