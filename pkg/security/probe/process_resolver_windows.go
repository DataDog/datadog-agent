// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package probe

import (
	"sync"

	"github.com/hashicorp/golang-lru/simplelru"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const (
	doForkListInput uint64 = iota
	doForkStructInput
)

const (
	snapshotting = iota
	snapshotted
)

const (
	procResolveMaxDepth = 16
	maxArgsEnvResidents = 1024
	maxParallelArgsEnvs = 512 // == number of parallel starting processes
)

// ProcessResolverOpts options of resolver
type ProcessResolverOpts struct {
	envsWithValue map[string]bool
}

// ProcessResolver resolved process context
type ProcessResolver struct {
	sync.RWMutex
	state     *atomic.Int64
	probe     *Probe
	resolvers *Resolvers
	cacheSize *atomic.Int64
	opts      ProcessResolverOpts

	// stats
	hitsStats      map[string]*atomic.Int64
	missStats      *atomic.Int64
	addedEntries   *atomic.Int64
	flushedEntries *atomic.Int64
	pathErrStats   *atomic.Int64
	argsTruncated  *atomic.Int64
	argsSize       *atomic.Int64
	envsTruncated  *atomic.Int64
	envsSize       *atomic.Int64

	entryCache    map[uint32]*model.ProcessCacheEntry
	argsEnvsCache *simplelru.LRU

	//argsEnvsPool          *ArgsEnvsPool
	//processCacheEntryPool *ProcessCacheEntryPool

	exitedQueue []uint32
}

func (p *ProcessResolver) Resolve(pid, tid uint32) *model.ProcessCacheEntry {
	return nil
}
