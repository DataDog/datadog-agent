// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package ndm provides Network Device Monitoring check configs from Remote
// Configuration.
package ndm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/ndm/handler"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// Provider is the Remote Configuration listener for the NDM product and the
// autodiscovery streaming config provider for the checks it produces.
type Provider struct {
	log      log.Component
	handlers map[string]handler.Handler
	keys     []string // registered keys, sorted, for deterministic iteration

	stateMutex sync.RWMutex // guards activeByPath, configErrors and closed

	activeByPath map[string]map[string][]integration.Config // path -> key -> configs
	configErrors map[string]types.ErrorMsgSet

	configChanges chan integration.ConfigChanges
	shutdownCh    chan struct{}
	closeOnce     sync.Once
	closed        bool
}

var _ types.StreamingConfigProvider = (*Provider)(nil)

// NewProvider builds the provider from the registered handlers, one per
// document key. It fails if two handlers claim the same key.
func NewProvider(logComp log.Component, handlers []handler.Handler) (*Provider, error) {
	byKey := make(map[string]handler.Handler, len(handlers))
	for _, h := range handlers {
		if h == nil {
			// An fx value group yields a zero value for a declining constructor.
			continue
		}
		key := h.Key()
		if key == "" {
			return nil, errors.New("an NDM remote configuration handler reports an empty key")
		}
		if _, exists := byKey[key]; exists {
			return nil, fmt.Errorf("two NDM remote configuration handlers claim the key %q", key)
		}
		byKey[key] = h
	}

	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Autodiscovery expects an initial value on the stream.
	configChanges := make(chan integration.ConfigChanges, 10)
	configChanges <- integration.ConfigChanges{}

	return &Provider{
		log:           logComp,
		handlers:      byKey,
		keys:          keys,
		activeByPath:  make(map[string]map[string][]integration.Config),
		configErrors:  make(map[string]types.ErrorMsgSet),
		configChanges: configChanges,
		shutdownCh:    make(chan struct{}),
	}, nil
}

// String returns the provider name.
func (p *Provider) String() string {
	return names.NDMRemoteConfig
}

// RegisteredKeys returns the document keys this provider dispatches, sorted.
func (p *Provider) RegisteredKeys() []string {
	return p.keys
}

// GetConfigErrors returns the last update's errors, indexed by config path.
func (p *Provider) GetConfigErrors() map[string]types.ErrorMsgSet {
	p.stateMutex.RLock()
	defer p.stateMutex.RUnlock()

	errorsByPath := make(map[string]types.ErrorMsgSet, len(p.configErrors))
	maps.Copy(errorsByPath, p.configErrors)
	return errorsByPath
}

// Stream sends NDM config changes to autodiscovery until ctx is cancelled.
func (p *Provider) Stream(ctx context.Context) <-chan integration.ConfigChanges {
	go func() {
		<-ctx.Done()
		p.close()
	}()
	return p.configChanges
}

func (p *Provider) close() {
	p.closeOnce.Do(func() {
		// Before the lock: sendChanges can block on a full channel holding the read lock.
		close(p.shutdownCh)

		p.stateMutex.Lock()
		defer p.stateMutex.Unlock()
		p.closed = true
		close(p.configChanges)
	})
}

// Update handles a complete NDM Remote Configuration snapshot, emitting all
// of its changes as one ConfigChanges. Not safe to call concurrently with
// itself.
func (p *Provider) Update(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	p.stateMutex.Lock()

	changes := integration.ConfigChanges{}
	seenPaths := make(map[string]struct{}, len(updates))

	for path, raw := range updates {
		keys, err := parseDocument(raw.Config)
		if err != nil {
			p.log.Debugf("ndm: ignoring config %s: %v", path, err)
			continue
		}

		configsByKey, errsByKey, owned := p.dispatch(path, keys)
		if !owned {
			p.log.Debugf("ndm: ignoring config %s: it carries no NDM key, only %v", path, documentKeys(keys))
			continue
		}

		// Only an owned path is seen, so it never sweeps another feature's state.
		seenPaths[path] = struct{}{}

		if set := errorSet(errsByKey, p.keys); set != nil {
			p.configErrors[path] = set
		} else {
			delete(p.configErrors, path)
		}
		applyStateCallback(path, applyStatus(errsByKey, p.keys))

		p.diffPath(path, configsByKey, &changes)
	}

	for path := range p.activeByPath {
		if _, found := seenPaths[path]; found {
			continue
		}
		p.removePath(path, &changes)
	}
	for path := range p.configErrors {
		if _, found := seenPaths[path]; found {
			continue
		}
		delete(p.configErrors, path)
	}

	scheduled, unscheduled := len(changes.Schedule), len(changes.Unschedule)
	p.stateMutex.Unlock()

	if scheduled > 0 || unscheduled > 0 {
		p.log.Infof("ndm: scheduling %d and unscheduling %d check configs across %d configs", scheduled, unscheduled, len(seenPaths))
	}
	p.sendChanges(changes)
}

// diffPath replaces a path's configs with the ones its handlers returned,
// recording only what changed. The caller must hold p.stateMutex for writing.
func (p *Provider) diffPath(path string, configsByKey map[string][]integration.Config, changes *integration.ConfigChanges) {
	current := p.activeByPath[path]

	for _, key := range p.keys {
		existing, held := current[key]
		replacement, produced := configsByKey[key]

		if held && sameConfigs(existing, replacement) {
			continue
		}
		if held {
			changes.Unschedule = append(changes.Unschedule, existing...)
		}
		if produced {
			changes.Schedule = append(changes.Schedule, replacement...)
		}
	}

	if len(configsByKey) == 0 {
		delete(p.activeByPath, path)
		return
	}
	p.activeByPath[path] = configsByKey
}

// removePath drops everything held for a path that left the configuration.
// The caller must hold p.stateMutex for writing.
func (p *Provider) removePath(path string, changes *integration.ConfigChanges) {
	for _, configs := range p.activeByPath[path] {
		changes.Unschedule = append(changes.Unschedule, configs...)
	}
	delete(p.activeByPath, path)
	delete(p.configErrors, path)
}

func (p *Provider) sendChanges(changes integration.ConfigChanges) {
	if changes.IsEmpty() {
		return
	}

	p.stateMutex.RLock()
	defer p.stateMutex.RUnlock()
	if p.closed {
		return
	}
	select {
	case p.configChanges <- changes:
	case <-p.shutdownCh:
	}
}

// sameConfigs reports whether two config sets are identical.
func sameConfigs(a, b []integration.Config) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name ||
			a[i].Source != b[i].Source ||
			a[i].FastDigest() != b[i].FastDigest() {
			return false
		}
	}
	return true
}

// documentKeys renders a document's top-level keys, sorted, for a log line.
type documentKeys map[string]json.RawMessage

func (d documentKeys) String() string {
	present := make([]string, 0, len(d))
	for key := range d {
		present = append(present, key)
	}
	sort.Strings(present)
	return strings.Join(present, ", ")
}
