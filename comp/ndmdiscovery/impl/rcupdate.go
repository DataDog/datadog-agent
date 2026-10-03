// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"sync"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// rcHandler applies Remote Configuration snapshots to the scheduler.
type rcHandler struct {
	sched    *scheduler
	log      log.Component
	defaults rangeDefaults

	mu sync.Mutex
	// activeByPath maps an RC config path to the autodiscovery ID this
	// component scheduled from it. Only claimed paths appear here.
	activeByPath map[string]string
}

func newRCHandler(sched *scheduler, logger log.Component, defaults rangeDefaults) *rcHandler {
	return &rcHandler{
		sched:        sched,
		log:          logger,
		defaults:     defaults,
		activeByPath: map[string]string{},
	}
}

// Update applies one Remote Configuration snapshot.
//
// The updates map is the complete set of configs for the NDM product, so a
// path that is absent has been deleted. The product is shared with other NDM
// subscribers, which is why a config of a foreign kind is skipped without a
// log line and without an applyStateCallback: acknowledging someone else's
// config would report a state this component cannot honour.
func (h *rcHandler) Update(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	h.mu.Lock()
	defer h.mu.Unlock()

	seenPaths := make(map[string]struct{}, len(updates))

	for path, raw := range updates {
		seenPaths[path] = struct{}{}

		kind, err := configKind(raw.Config)
		if err != nil {
			// The kind is unreadable, so this config cannot be claimed.
			h.dropClaim(path, "its payload is no longer readable")
			continue
		}
		if kind != kindAutodiscovery {
			h.dropClaim(path, "it now carries a kind this component does not own")
			continue
		}

		cfg, err := parseRangeConfig(raw.Config, h.defaults)
		if err != nil {
			h.reject(path, applyStateCallback, err)
			continue
		}

		if err := h.sched.set(cfg); err != nil {
			h.reject(path, applyStateCallback, err)
			continue
		}

		// A path keeps its RC path while its payload names a different range.
		// The range it used to name has no config describing it any more, so
		// it is released here rather than left sweeping forever.
		if previousID, ok := h.activeByPath[path]; ok && previousID != cfg.AutodiscoveryID {
			h.log.Infof("ndmdiscovery: config %s now names range %s, stopping range %s", path, cfg.AutodiscoveryID, previousID)
			h.releasePath(path)
		}

		h.activeByPath[path] = cfg.AutodiscoveryID
		applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}

	for path, autodiscoveryID := range h.activeByPath {
		if _, ok := seenPaths[path]; ok {
			continue
		}
		h.log.Infof("ndmdiscovery: range %s was removed from the configuration", autodiscoveryID)
		h.releasePath(path)
	}
}

// reject stops whatever this path was running and reports the reason to the
// backend, so an operator sees why their range is not being scanned.
func (h *rcHandler) reject(path string, applyStateCallback func(string, state.ApplyStatus), err error) {
	h.log.Warnf("ndmdiscovery: rejecting autodiscovery config %s: %v", path, err)
	h.releasePath(path)
	applyStateCallback(path, state.ApplyStatus{
		State: state.ApplyStateError,
		Error: err.Error(),
	})
}

// dropClaim releases a range this component had claimed from a path whose
// payload it can no longer claim, either because the payload is unreadable or
// because it now belongs to another NDM subscriber.
//
// No applyStateCallback is reported: the product is shared, and acknowledging
// a config this component cannot claim would report a state it cannot honour.
// The range still has to stop, because nothing describes it any more.
func (h *rcHandler) dropClaim(path, reason string) {
	id, ok := h.activeByPath[path]
	if !ok {
		return
	}
	h.log.Warnf("ndmdiscovery: stopping range %s from config %s: %s", id, path, reason)
	h.releasePath(path)
}

// releasePath forgets a path and stops the range it named.
//
// The scheduler is keyed by autodiscovery ID while activeByPath is keyed by RC
// path, so two paths can name the same ID. Removing the ID whenever one of
// those paths goes away would stop a range the other path still asks for,
// which is why the removal only happens once no path references the ID.
func (h *rcHandler) releasePath(path string) {
	id, ok := h.activeByPath[path]
	if !ok {
		return
	}
	delete(h.activeByPath, path)
	for _, otherID := range h.activeByPath {
		if otherID == id {
			return
		}
	}
	h.sched.remove(id)
}
