// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

package process

import (
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/mount"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/path"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/usergroup"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	stime "github.com/DataDog/datadog-agent/pkg/util/ktime"
)

// NewTestEBPFResolver creates a minimal EBPFResolver suitable for testing.
// It initialises all unexported fields that are accessed during event processing
// (caches, atomic counters, maps) so that methods like UpdateArgsEnvs, AddForkEntry,
// AddExecEntry, Resolve, ApplyExitEntry, UpdateUID, UpdateGID, etc. do not panic.
func NewTestEBPFResolver(
	timeResolver *stime.Resolver,
	pathResolver path.ResolverInterface,
	mountResolver mount.ResolverInterface,
	userGroupResolver *usergroup.Resolver,
) (*EBPFResolver, error) {
	argsEnvsCache, err := simplelru.NewLRU[uint64, *argsEnvsCacheEntry](maxParallelArgsEnvs, nil)
	if err != nil {
		return nil, err
	}

	p := &EBPFResolver{
		state:                     atomic.NewInt64(Snapshotting),
		entryCache:                make(map[uint32]*model.ProcessCacheEntry),
		argsEnvsCache:             argsEnvsCache,
		timeResolver:              timeResolver,
		pathResolver:              pathResolver,
		mountResolver:             mountResolver,
		userGroupResolver:         userGroupResolver,
		hitsStats:                 make(map[string]*atomic.Int64),
		missStats:                 atomic.NewInt64(0),
		addedEntriesFromEvent:     atomic.NewInt64(0),
		addedEntriesFromKernelMap: atomic.NewInt64(0),
		addedEntriesFromProcFS:    atomic.NewInt64(0),
		flushedEntries:            atomic.NewInt64(0),
		pathErrStats:              atomic.NewInt64(0),
		argsTruncated:             atomic.NewInt64(0),
		argsSize:                  atomic.NewInt64(0),
		envsTruncated:             atomic.NewInt64(0),
		envsSize:                  atomic.NewInt64(0),
		brokenLineage:             atomic.NewInt64(0),
		inodeErrStats:             make(map[string]*atomic.Int64),
	}

	for _, t := range metrics.AllTypesTags {
		p.hitsStats[t] = atomic.NewInt64(0)
	}

	for _, tag := range allInodeErrTags() {
		p.inodeErrStats[tag] = atomic.NewInt64(0)
	}

	return p, nil
}
