// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package hardening

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// configs is one source's policies and exception shards.
type configs struct {
	policies   map[Control]*Policy
	exceptions map[exceptionsKey]*Exceptions
}

type exceptionsKey struct {
	control    Control
	generation int64
	shard      int
}

func newConfigs() configs {
	return configs{policies: map[Control]*Policy{}, exceptions: map[exceptionsKey]*Exceptions{}}
}

// add returns false when another config already holds the policy of its control.
func (c configs) add(p *Policy, e *Exceptions) bool {
	if p != nil {
		if _, dup := c.policies[p.Control]; dup {
			return false
		}
		c.policies[p.Control] = p
	}
	if e != nil {
		c.exceptions[exceptionsKey{e.Control, e.Generation, e.Shard}] = e
	}
	return true
}

// Store holds the hardening configs known to this Cluster Agent replica. Every
// replica keeps one, because the API server may send an admission request to
// any of them. Remote Config wins over the configs file, control by control.
type Store struct {
	clusterID string

	mu   sync.RWMutex
	rc   configs
	file configs

	changes chan struct{}
}

// NewStore returns an empty Store for the cluster whose ID is clusterID.
func NewStore(clusterID string) *Store {
	return &Store{clusterID: clusterID, rc: newConfigs(), file: newConfigs(), changes: make(chan struct{}, 1)}
}

// ClusterID returns the ID of this cluster.
func (s *Store) ClusterID() string {
	return s.clusterID
}

// Changes is signalled after every update.
func (s *Store) Changes() <-chan struct{} {
	return s.changes
}

// Policies returns every policy, sorted by control.
func (s *Store) Policies() []*Policy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Policy
	for control, p := range s.file.policies {
		if _, ok := s.rc.policies[control]; !ok {
			out = append(out, p)
		}
	}
	for _, p := range s.rc.policies {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Control < out[j].Control })
	return out
}

// Policy returns the policy with this id.
func (s *Store) Policy(id string) (*Policy, bool) {
	for _, p := range s.Policies() {
		if p.ID == id {
			return p, true
		}
	}
	return nil, false
}

// Exceptions returns the newest exceptions generation of control, at least
// minGeneration, whose every shard is loaded. It returns false when there is
// none, so a missing shard never means harden everything.
func (s *Store) Exceptions(control Control, minGeneration int64) (*ExceptionSet, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	source := s.file
	if _, ok := s.rc.policies[control]; ok {
		source = s.rc
	}
	byGeneration := map[int64][]*Exceptions{}
	for k, e := range source.exceptions {
		if k.control == control && k.generation >= minGeneration {
			byGeneration[k.generation] = append(byGeneration[k.generation], e)
		}
	}
	var best []*Exceptions
	for _, shards := range byGeneration {
		if complete(shards) && (best == nil || shards[0].Generation > best[0].Generation) {
			best = shards
		}
	}
	if best == nil {
		return nil, false
	}
	return mergeShards(best), true
}

// complete reports whether shards holds every shard of its generation.
func complete(shards []*Exceptions) bool {
	n := shards[0].Shards
	seen := map[int]bool{}
	for _, s := range shards {
		if s.Shards != n {
			return false
		}
		seen[s.Shard] = true
	}
	return len(seen) == n
}

func (s *Store) set(fromRC bool, c configs) {
	s.mu.Lock()
	if fromRC {
		s.rc = c
	} else {
		s.file = c
	}
	s.mu.Unlock()
	select {
	case s.changes <- struct{}{}:
	default:
	}
}

// OnRCUpdate is the Remote Config callback. Each update carries every config of
// the product, so it replaces the Remote Config configs wholesale.
func (s *Store) OnRCUpdate(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	c := newConfigs()
	paths := make([]string, 0, len(updates))
	for path := range updates {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		p, e, err := parseConfig(updates[path].Config, s.clusterID)
		if err == nil && !c.add(p, e) {
			err = errDuplicatePolicy
		}
		if err != nil {
			log.Warnf("hardening: ignoring remote config %s: %v", path, err)
			applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}
		applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}
	s.set(true, c)
}

var errDuplicatePolicy = errors.New("another config already holds the policy of this control")

// setFileConfigs replaces the file configs with data, a JSON array of
// configs. Invalid entries are skipped; invalid JSON keeps the previous set.
func (s *Store) setFileConfigs(data []byte) error {
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return err
	}
	c := newConfigs()
	for _, raw := range raws {
		p, e, err := parseConfig(raw, s.clusterID)
		if err == nil && !c.add(p, e) {
			err = errDuplicatePolicy
		}
		if err != nil {
			log.Warnf("hardening: ignoring entry in the configs file: %v", err)
		}
	}
	s.set(false, c)
	return nil
}

// WatchFile loads path now, then again every interval when its content changes,
// until ctx is done.
// ponytail: polling, as the file is a dev/testing feed; use fsnotify if it ever matters.
func (s *Store) WatchFile(ctx context.Context, path string, interval time.Duration) {
	var last []byte
	var lastErr string
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		data, err := os.ReadFile(path)
		switch {
		case err != nil:
			if err.Error() != lastErr {
				log.Warnf("hardening: cannot read configs file: %v", err)
				lastErr = err.Error()
			}
		case !bytes.Equal(data, last):
			if err := s.setFileConfigs(data); err != nil {
				log.Warnf("hardening: keeping previous configs, %s is invalid: %v", path, err)
			}
			last, lastErr = data, ""
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
