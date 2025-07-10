// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package file holds file related files
package file

import (
	"fmt"
	"os"
	"syscall"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/config"
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
	Enabled bool

	cache *lru.Cache[LRUCacheKey, *model.FileMetadata]

	cgroupResolver *cgroup.Resolver

	// stats
	statsdClient statsd.ClientInterface
	cacheHit     *atomic.Uint64
	cacheMiss    *atomic.Uint64
}

// LRUCacheKey is the structure used to access cached metadata
type LRUCacheKey struct {
	containerID containerutils.ContainerID
	path        string
	mTime       int64
}

// NewResolver returns a new instance of the hash resolver
func NewResolver(cfg *config.RuntimeSecurityConfig, statsdClient statsd.ClientInterface, opt *Opt) (*Resolver, error) {
	if !cfg.FileMetadataResolverEnabled {
		return &Resolver{
			Enabled: false,
		}, nil
	}

	cache, err := lru.New[LRUCacheKey, *model.FileMetadata](fileMetadataResolverCacheSize)
	if err != nil {
		return nil, fmt.Errorf("couldn't create file metadata resolver cache: %w", err)
	}

	return &Resolver{
		Enabled:        true,
		statsdClient:   statsdClient,
		cgroupResolver: opt.CgroupResolver,
		cache:          cache,
		cacheHit:       atomic.NewUint64(0),
		cacheMiss:      atomic.NewUint64(0),
	}, nil
}

// ResolveFileMetadata resolves file metadata
func (r *Resolver) ResolveFileMetadata(event *model.Event, file *model.FileEvent) (*model.FileMetadata, error) {
	if !r.Enabled {
		return nil, nil
	}
	if !file.IsPathnameStrResolved {
		event.FieldHandlers.ResolveFilePath(event, file)
	}
	if file.PathResolutionError != nil {
		return nil, fmt.Errorf("path resolution error: %w", file.PathResolutionError)
	}

	// fileless
	if file.BasenameStr == "memfd:" && file.PathnameStr == "" {
		return &model.FileMetadata{
			Type:         int(model.FileLess),
			IsExecutable: true,
		}, nil
	}

	// add pid one for hash resolution outside of a container
	process := event.ProcessContext.Process
	rootPIDs := []uint32{process.Pid, 1}
	if process.ContainerID != "" && r.cgroupResolver != nil {
		w, ok := r.cgroupResolver.GetWorkload(process.ContainerID)
		if ok {
			rootPIDs = w.GetPIDs()
		}
	} else if event.ProcessCacheEntry != nil {
		rootPIDs = event.ProcessCacheEntry.GetAncestorsPIDs()
	}

	for _, pid := range rootPIDs {
		//  get proc path
		path := utils.ProcRootFilePath(pid, file.PathnameStr)

		// get file stats
		fileInfo, err := os.Stat(path)
		if err != nil {
			if os.IsPermission(err) {
				return nil, err
			}
			continue
		}
		if stat, ok := fileInfo.Sys().(*syscall.Stat_t); ok && stat.Ino != file.Inode {
			return nil, fmt.Errorf("file %s have the inode %d, but we are looking for inode %d", path, stat.Ino, file.Inode)
		}

		// validate that it's a regular file
		if !fileInfo.Mode().IsRegular() {
			return nil, fmt.Errorf("file %s is not a regular file", path)
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
			if os.IsPermission(err) {
				return nil, err
			}
			continue
		}

		r.cacheMiss.Inc()
		r.cache.Add(key, info)
		return info, nil
	}
	return nil, fmt.Errorf("failed to analyze file metadata for %s", file.PathnameStr)
}

// SendStats sends the resolver metrics
func (r *Resolver) SendStats() error {
	if r.statsdClient == nil || !r.Enabled {
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
