// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds security profiles related files
package securityprofile

import (
	"errors"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"
)

func (m *Manager) newProcessCacheEntrySearcher(ad *dump.ActivityDump) *processCacheEntrySearcher {
	return &processCacheEntrySearcher{
		manager:       m,
		ad:            ad,
		ancestorCache: make(map[*model.ProcessContext]*model.ProcessCacheEntry),
	}
}

// updateTracedPid traces a pid in kernel space
func (m *Manager) updateTracedPid(ad *dump.ActivityDump, pid uint32) {
	// start by looking up any existing entry
	var cookie uint64
	_ = m.tracedPIDsMap.Lookup(pid, &cookie)
	if cookie != ad.Cookie {
		config := ad.LoadConfig.Load()
		_ = m.tracedPIDsMap.Put(pid, &config)
	}
}

type processCacheEntrySearcher struct {
	manager       *Manager
	ad            *dump.ActivityDump
	ancestorCache map[*model.ProcessContext]*model.ProcessCacheEntry
}

func (pces *processCacheEntrySearcher) getNextAncestorBinaryOrArgv0(pc *model.ProcessContext) *model.ProcessCacheEntry {
	if ancestor, ok := pces.ancestorCache[pc]; ok {
		return ancestor
	}
	newAncestor := activity_tree.GetNextAncestorBinaryOrArgv0(pc)
	pces.ancestorCache[pc] = newAncestor
	return newAncestor
}

// SearchTracedProcessCacheEntry inserts traced pids if necessary
func (pces *processCacheEntrySearcher) searchTracedProcessCacheEntry(entry *model.ProcessCacheEntry) {
	// check process lineage
	if !pces.ad.MatchesSelector(entry) {
		return
	}

	if _, err := entry.HasValidLineage(); err != nil {
		// check if the node belongs to the container
		var mn *model.ErrProcessMissingParentNode
		if !errors.As(err, &mn) {
			return
		}
	}

	// compute the list of ancestors, we need to start inserting them from the root
	ancestors := []*model.ProcessCacheEntry{entry}
	parent := pces.getNextAncestorBinaryOrArgv0(&entry.ProcessContext)
	for parent != nil && pces.ad.MatchesSelector(parent) {
		ancestors = append(ancestors, parent)
		parent = pces.getNextAncestorBinaryOrArgv0(&parent.ProcessContext)
	}
	slices.Reverse(ancestors)

	pces.ad.Profile.AddSnapshotAncestors(
		ancestors,
		pces.manager.resolvers,
		func(pce *model.ProcessCacheEntry) {
			pces.manager.updateTracedPid(pces.ad, pce.Process.Pid)
		},
	)
}

func (m *Manager) snapshot(ad *dump.ActivityDump) error {
	ad.Profile.Snapshot(m.newEvent)

	// try to resolve the tags now
	_ = m.resolveTags(ad)
	return nil
}
