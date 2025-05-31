// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package file holds file related files
package file

import (
	"errors"
	"fmt"
	"os"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/security/utils/lru"
	"github.com/DataDog/datadog-go/v5/statsd"
)

const (
	checkLinkage                  = false
	fileMetadataResolverCacheSize = 512
)

// Opt defines options for resolvers
type Opt struct {
	CgroupResolver *cgroup.Resolver
}

// Resolver represents a cache for mountpoints and the corresponding file systems
type Resolver struct {
	cache *lru.Cache[LRUCacheKey, *model.FileMetadatas]

	cgroupResolver *cgroup.Resolver

	// stats
	statsdClient statsd.ClientInterface
	cacheHit     *atomic.Uint64
	cacheMiss    *atomic.Uint64
}

// LRUCacheKey is the structure used to access cached metadatas
type LRUCacheKey struct {
	containerID containerutils.ContainerID
	path        string
	mTime       int64
}

// NewResolver returns a new instance of the hash resolver
func NewResolver(statsdClient statsd.ClientInterface, opt *Opt) (*Resolver, error) {
	cache, err := lru.New[LRUCacheKey, *model.FileMetadatas](fileMetadataResolverCacheSize)
	if err != nil {
		return nil, fmt.Errorf("couldn't create file metadatas resolver cache: %w", err)
	}

	return &Resolver{
		statsdClient:   statsdClient,
		cgroupResolver: opt.CgroupResolver,
		cache:          cache,
		cacheHit:       atomic.NewUint64(0),
		cacheMiss:      atomic.NewUint64(0),
	}, nil
}

// ResolveFileMetadatas resolves file metadatas
func (r *Resolver) ResolveFileMetadatas(event *model.Event, file *model.FileEvent) (*model.FileMetadatas, error) {
	if !file.IsPathnameStrResolved {
		event.FieldHandlers.ResolveFilePath(event, file)
	}
	if file.PathResolutionError != nil {
		return nil, errors.New("path resolution error")
	}

	// fileless
	if file.BasenameStr == "memfd:" && file.PathnameStr == "" {
		return &model.FileMetadatas{
			Type:         int(model.FileLess),
			IsExecutable: true,
		}, nil
	}

	// add pid one for hash resolution outside of a container
	process := event.ProcessContext.Process
	rootPIDs := []uint32{process.Pid, 1}
	if event.ProcessCacheEntry != nil {
		rootPIDs = event.ProcessCacheEntry.GetAncestorsPIDs()
	}
	if process.ContainerID != "" && r.cgroupResolver != nil {
		w, ok := r.cgroupResolver.GetWorkload(process.ContainerID)
		if ok {
			rootPIDs = w.GetPIDs()
		}
	}

	for _, pid := range rootPIDs {
		//  get proc path
		path := utils.ProcRootFilePath(pid, file.PathnameStr)

		// get file stats
		fileInfo, err := os.Stat(path)
		if err != nil {
			continue
		}

		// init the cache key to lookup
		key := LRUCacheKey{
			containerID: process.ContainerID,
			path:        path,
			mTime:       fileInfo.ModTime().UnixNano(),
		}

		// cache lookup
		entry, ok := r.cache.Get(key)
		if ok {
			r.cacheHit.Inc()
			return entry, nil
		}

		// if no result in cache, analyze the file an return result
		info, err := AnalyzeFile(path, fileInfo, checkLinkage)
		if err != nil {
			continue
		}

		r.cacheMiss.Inc()
		r.cache.Add(key, info)
		return info, nil
	}
	return nil, fmt.Errorf("failed to analyze file metadatas for %s", file.PathnameStr)
}

// SendStats sends the resolver metrics
func (r *Resolver) SendStats() error {
	if r.statsdClient == nil {
		return nil
	}

	hits := r.cacheHit.Swap(0)
	if err := r.statsdClient.Count(metrics.MetricFileResolverCacheHit, int64(hits), []string{}, 1.0); err != nil {
		return fmt.Errorf("couldn't send MetricFileResolverCacheHit metric: %w", err)
	}

	misses := r.cacheMiss.Swap(0)
	if err := r.statsdClient.Count(metrics.MetricFileResolverCacheMiss, int64(misses), []string{}, 1.0); err != nil {
		return fmt.Errorf("couldn't send MetricFileResolverCacheMiss metric: %w", err)
	}

	return nil
}
