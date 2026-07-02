// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package integrationdetection detects which integrations are enabled on a host
// by subscribing to Autodiscovery schedule events.
package integrationdetection

import (
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// Scope indicates whether an integration instance runs on the host or in a container.
type Scope string

const (
	// ScopeHost is for integrations running on the host (file-based or process-based AD).
	ScopeHost Scope = "host"
	// ScopeContainer is for integrations running inside a container.
	ScopeContainer Scope = "container"
)

// EnabledIntegration is one scheduled integration instance detected by the Detector.
// Multiple entries with the same Integration value may exist when a host runs more
// than one instance of the same service (e.g. two Redis ports). Each is uniquely
// identified by its Digest, which is the AD config digest.
type EnabledIntegration struct {
	Integration string // canonical name, e.g. "redis"
	CheckName   string // AD check name, e.g. "redisdb"
	Scope       Scope
	Runtime     string // container runtime, e.g. "docker" (empty when Scope=host)
	ContainerID string // (empty when Scope=host)
	Digest      string // config.Digest(); uniquely identifies this instance
}

// Detector subscribes to Autodiscovery schedule events and maintains a live
// snapshot of enabled integrations on the host.
//
// Lifecycle: Start must be called once to begin receiving events; Stop must be
// called at most once to deregister. Start and Stop must NOT be called
// concurrently — the Fx lifecycle contract (sequential OnStart/OnStop) provides
// this guarantee in production. sync.Once guards enforce idempotency.
//
// A Detector must not be copied after first use (it embeds sync.RWMutex and
// sync.Once).
type Detector struct {
	mu        sync.RWMutex
	active    map[string]EnabledIntegration // keyed by config.Digest()
	stopped   bool                          // set under mu before RemoveScheduler; guards stale post-Stop callbacks
	ac        autodiscovery.Component       // written exactly once by Start's startOnce.Do (under mu); read exactly once by Stop's stopOnce.Do (under mu)
	startOnce sync.Once
	stopOnce  sync.Once
}

// NewDetector creates a Detector. Call Start to begin receiving AD events.
func NewDetector() *Detector {
	return &Detector{
		active: make(map[string]EnabledIntegration),
	}
}

// adScheduler bridges the scheduler.Scheduler interface to the Detector.
type adScheduler struct{ d *Detector }

func (s *adScheduler) Schedule(cfgs []integration.Config)   { s.d.schedule(cfgs) }
func (s *adScheduler) Unschedule(cfgs []integration.Config) { s.d.unschedule(cfgs) }

// Stop is an intentional no-op; must NOT forward to Detector.Stop — doing so
// would create a double-stop path since the AD controller calls this when
// deregistering. Use Detector.Stop() directly to deregister from AD.
func (s *adScheduler) Stop() {}

// Start registers with the AD controller and replays any already-scheduled configs.
// Must be called from an Fx OnStart hook, not the constructor, so that the AD
// controller has finished initialising before registration. Calling Start more
// than once is a no-op (guarded by sync.Once).
//
// Start and Stop must not be called concurrently; the Fx lifecycle guarantees
// this in production.
func (d *Detector) Start(ac autodiscovery.Component) {
	d.startOnce.Do(func() {
		d.mu.Lock()
		d.ac = ac
		stopped := d.stopped // capture under the same lock — no second round-trip needed
		d.mu.Unlock()

		ac.AddScheduler("integration-detection", &adScheduler{d}, true)

		// If Stop ran concurrently during AddScheduler, the scheduler was
		// registered after RemoveScheduler was called. Detect this and
		// immediately deregister to leave the detector in a clean stopped state.
		// RemoveScheduler is idempotent (no-op on unknown names) so a double-call
		// from the normal Stop path followed by this one is safe.
		if stopped {
			ac.RemoveScheduler("integration-detection")
		}
	})
}

// Stop deregisters from the AD controller and clears internal state. Calling
// Stop more than once is a no-op (guarded by sync.Once).
//
// The stopped flag is set and d.ac is captured under the mutex in a single
// critical section. This prevents a data race with concurrent Start callers and
// ensures any Schedule callback that starts executing concurrently will see
// stopped=true and bail out before writing to d.active.
func (d *Detector) Stop() {
	d.stopOnce.Do(func() {
		d.mu.Lock()
		d.stopped = true
		ac := d.ac
		clear(d.active) // reuse map memory rather than allocating a new one
		d.mu.Unlock()

		if ac != nil {
			ac.RemoveScheduler("integration-detection")
		}
		// Note: any Schedule callback that acquired the write lock *before* Stop
		// set d.stopped will have added an entry that is now cleared. This is
		// intentional — the agent is shutting down and the data is discarded.
	})
}

// Snapshot returns a copy of the currently enabled integrations, or nil when
// no integrations are detected. The order of entries in the returned slice is
// not defined. May return multiple entries with the same Integration value when
// the host runs multiple instances of the same service.
func (d *Detector) Snapshot() []EnabledIntegration {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return slices.Collect(maps.Values(d.active))
}

// schedule handles Schedule events from the AD controller.
// Callbacks may arrive from the AD goroutine and must not block for extended periods.
//
// Note on Stop() interaction: Stop() sets d.stopped=true and calls RemoveScheduler
// while holding the lock for the flag and clear operations (but not for
// RemoveScheduler itself). A callback that arrives after d.stopped=true is set
// will see the flag under the lock and return early, making the window safe.
func (d *Detector) schedule(cfgs []integration.Config) {
	// cfg.Digest() does YAML unmarshal+marshal + murmur3 hash per instance.
	// Build the entries before acquiring the lock so the write-lock hold time
	// covers only map writes, not per-config hashing and string ops.
	type entry struct {
		digest string
		ei     EnabledIntegration
	}
	var entries []entry
	for _, cfg := range cfgs {
		if !cfg.IsCheckConfig() {
			continue
		}
		canonical, ok := integrationForCheck(cfg.Name)
		if !ok {
			continue
		}
		scope, runtime, containerID := classifyServiceID(cfg.ServiceID)
		digest := cfg.Digest()
		entries = append(entries, entry{digest, EnabledIntegration{
			Integration: canonical,
			CheckName:   cfg.Name,
			Scope:       scope,
			Runtime:     runtime,
			ContainerID: containerID,
			Digest:      digest,
		}})
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return
	}
	for _, e := range entries {
		d.active[e.digest] = e.ei
	}
}

// unschedule handles Unschedule events from the AD controller.
// Callbacks may arrive from the AD goroutine and must not block for extended periods.
func (d *Detector) unschedule(cfgs []integration.Config) {
	// Precompute digests outside the lock — same reasoning as schedule().
	// No stopped guard needed: delete on a cleared map is a no-op.
	digests := make([]string, len(cfgs))
	for i, cfg := range cfgs {
		digests[i] = cfg.Digest()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, digest := range digests {
		delete(d.active, digest)
	}
}

// classifyServiceID maps an AD ServiceID to its scope, runtime, and container ID.
// There are three distinct forms:
//   - "" → file-based static config (conf.d/); host scope
//   - "process://<pid>" → process-based AD listener; host scope
//   - "<runtime>://<id>" → container listener; container scope
//
// The process:// prefix is checked explicitly before falling through to
// containers.SplitEntityName, which would otherwise misclassify it as a container
// (IsEntityName matches any "://" string). Malformed "://" strings where
// SplitEntityName returns an empty runtime or ID are treated as host scope.
//
// The function relies on containers.SplitEntityName returning ("", "") for any
// input that is not a valid entity name (i.e. any string without exactly one
// "://" separator). This is the trust boundary for the container classification:
// strings that look like runtime IDs but are not (e.g. bare process://pid) are
// caught by the explicit prefix checks above and never reach SplitEntityName.
//
// If a new AD service ID scheme is introduced (e.g. "kubernetes://..."), it
// must be explicitly enumerated above the SplitEntityName call to prevent
// misclassification as a container scope.
func classifyServiceID(serviceID string) (scope Scope, runtime, containerID string) {
	switch {
	case serviceID == "" || strings.HasPrefix(serviceID, "process://"):
		return ScopeHost, "", ""
	default:
		r, id := containers.SplitEntityName(serviceID)
		if r == "" || id == "" {
			return ScopeHost, "", ""
		}
		return ScopeContainer, r, id
	}
}
