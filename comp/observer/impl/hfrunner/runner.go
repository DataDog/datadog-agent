// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hfrunner

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"

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

	// hfSource is the observer handle source name for HF system check metrics.
	// Using a distinct name from "all-metrics" lets the observer suppress the
	// lower-frequency 15s versions of these metrics when HF mode is active.
	HFSource = "system-checks-hf"
)

// checkEntry tracks a single check instance and its retry state.
type checkEntry struct {
	name    string
	ch      check.Check
	backoff time.Duration // current backoff duration; 0 = no active backoff
	retryAt time.Time     // when to next attempt after a failure
	logged  bool          // whether we've already logged the first failure
}

// Runner runs system checks at 1-second intervals and routes their output
// directly into the observer pipeline. It is owned by the observer component
// and has no interaction with the normal collector/aggregator/forwarder chain.
type Runner struct {
	entries  []*checkEntry
	stopCh   chan struct{}
	stopOnce sync.Once
}

// New creates a Runner, instantiates system checks, and configures them.
// Checks that are unavailable on the current platform (e.g. battery on Linux
// servers, Windows-only checks on Linux) are silently skipped.
// Returns the runner; call Start() to begin collection.
func New(handle observerdef.Handle) *Runner {
	mgr := newObserverSenderManager(handle)

	// factories lists the system check factories to instantiate.
	// Each entry is (display-name, factory-option). Platform-specific factories
	// return an empty option on unsupported OSes, so we skip them gracefully.
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

		// Configure with empty instance/init config so each check uses its
		// defaults. We intentionally do not set min_collection_interval here
		// because the runner drives its own 1s tick loop independently.
		err := ch.Configure(mgr, 0, integration.Data("{}"), integration.Data("{}"), "hf-runner", "hf-runner")
		if err != nil {
			log.Warnf("[observer/hfrunner] failed to configure %s check, skipping: %v", fe.name, err)
			continue
		}

		entries = append(entries, &checkEntry{name: fe.name, ch: ch})
	}

	log.Infof("[observer/hfrunner] initialized %d system checks for high-frequency collection", len(entries))
	return &Runner{entries: entries, stopCh: make(chan struct{})}
}

// Start begins the 1-second collection loop in a background goroutine.
func (r *Runner) Start() {
	go r.run()
}

// Stop signals the collection loop to exit. Safe to call multiple times.
func (r *Runner) Stop() {
	r.stopOnce.Do(func() { close(r.stopCh) })
}

func (r *Runner) run() {
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

func (r *Runner) runCheck(e *checkEntry, now time.Time) {
	// Honour backoff: if we're in a retry window, skip this tick.
	if e.backoff > 0 && now.Before(e.retryAt) {
		return
	}

	if err := e.ch.Run(); err != nil {
		if !e.logged {
			// Log the first failure, then go quiet until the check recovers.
			log.Warnf("[observer/hfrunner] %s check failed (will retry with backoff): %v", e.name, err)
			e.logged = true
		}
		// Increase backoff exponentially, capped at maxBackoff.
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

	// Success: reset retry state and re-enable failure logging.
	if e.backoff > 0 {
		log.Infof("[observer/hfrunner] %s check recovered", e.name)
	}
	e.backoff = 0
	e.logged = false
}
