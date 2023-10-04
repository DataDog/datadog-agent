// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package usergroup holds usergroup related files
package usergroup

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"golang.org/x/time/rate"

	lru "github.com/hashicorp/golang-lru/v2"
)

const refreshCacheRateLimit = 10
const refreshCacheRateBurst = 40

var errUserNotFound = errors.New("user not found")
var errGroupNotFound = errors.New("group not found")

// EntryCache maps ids to names
type EntryCache struct {
	entries     map[int]string
	rateLimiter *rate.Limiter
}

// Resolver resolves user and group ids to names
type Resolver struct {
	cgroupResolver *cgroup.Resolver
	nsUserCache    *lru.Cache[string, *EntryCache]
	nsGroupCache   *lru.Cache[string, *EntryCache]
}

type containerFS struct {
	cgroup *cgroupModel.CacheEntry
}

// Open implements the fs.FS interface for containers
func (fs *containerFS) Open(filename string) (fs.File, error) {
	for _, rootCandidatePID := range fs.cgroup.GetPIDs() {
		file, err := os.Open(filepath.Join(utils.ProcRootPath(rootCandidatePID), filename))
		if err != nil {
			seclog.Errorf("failed to read %s for pid %d of container %s: %s", filename, rootCandidatePID, fs.cgroup.ID, err)
			continue
		}

		return file, nil
	}

	return nil, fmt.Errorf("failed to resolve root filesystem for %s", fs.cgroup.ID)
}

type hostFS struct{}

// Open implements the fs.FS interface for hosts
func (fs *hostFS) Open(name string) (fs.File, error) {
	passwdPath := "/etc/passwd"
	if hostRoot := os.Getenv("HOST_ROOT"); hostRoot != "" {
		passwdPath = filepath.Join(hostRoot, passwdPath)
	}
	return os.Open(passwdPath)
}

func (r *Resolver) getFilesystem(containerID string) (fs.FS, error) {
	var fsys fs.FS

	if containerID != "" {
		cgroupEntry, found := r.cgroupResolver.GetWorkload(containerID)
		if !found {
			return nil, fmt.Errorf("failed to resolve container %s", containerID)
		}
		fsys = &containerFS{cgroup: cgroupEntry}
	} else {
		fsys = &hostFS{}
	}

	return fsys, nil
}

// RefreshCache refresh the user and group caches with data from files
func (r *Resolver) RefreshCache(containerID string) error {
	fsys, err := r.getFilesystem(containerID)
	if err != nil {
		return err
	}

	if _, err := r.refreshUserCache(containerID, fsys); err != nil {
		return err
	}

	if _, err := r.refreshGroupCache(containerID, fsys); err != nil {
		return err
	}

	return nil
}

func (r *Resolver) refreshUserCache(containerID string, fsys fs.FS) (map[int]string, error) {
	entryCache, found := r.nsUserCache.Get(containerID)
	if !found {
		// add the entry cache before we parse the fill so that we also
		// rate limit parsing failures
		entryCache = &EntryCache{rateLimiter: rate.NewLimiter(rate.Limit(refreshCacheRateLimit), refreshCacheRateBurst)}
		r.nsUserCache.Add(containerID, entryCache)
	}

	if !entryCache.rateLimiter.Allow() {
		return entryCache.entries, nil
	}

	entries, err := parsePasswd(fsys, "/etc/passwd")
	if err != nil {
		return nil, err
	}
	entryCache.entries = entries

	return entries, nil
}

func (r *Resolver) refreshGroupCache(containerID string, fsys fs.FS) (map[int]string, error) {
	entryCache, found := r.nsGroupCache.Get(containerID)
	if !found {
		entryCache = &EntryCache{rateLimiter: rate.NewLimiter(rate.Limit(refreshCacheRateLimit), refreshCacheRateBurst)}
		r.nsGroupCache.Add(containerID, entryCache)
	}

	if !entryCache.rateLimiter.Allow() {
		return entryCache.entries, nil
	}

	entries, err := parseGroup(fsys, "/etc/group")
	if err != nil {
		return nil, err
	}
	entryCache.entries = entries

	return entries, nil
}

// ResolveUser resolves a user id to a username
func (r *Resolver) ResolveUser(uid int, containerID string) (string, error) {
	userCache, found := r.nsUserCache.Get(containerID)
	if found {
		cachedEntry, found := userCache.entries[uid]
		if !found {
			return "", errUserNotFound
		}
		return cachedEntry, nil
	}

	fsys, err := r.getFilesystem(containerID)
	if err != nil {
		return "", err
	}

	userEntries, err := r.refreshUserCache(containerID, fsys)
	if err != nil {
		return "", err
	}

	userName, found := userEntries[uid]
	if !found {
		return "", errUserNotFound
	}

	return userName, nil
}

// ResolveGroup resolves a group id to a group name
func (r *Resolver) ResolveGroup(gid int, containerID string) (string, error) {
	groupCache, found := r.nsGroupCache.Get(containerID)
	if found {
		cachedEntry, found := groupCache.entries[gid]
		if !found {
			return "", errGroupNotFound
		}
		return cachedEntry, nil
	}

	fsys, err := r.getFilesystem(containerID)
	if err != nil {
		return "", err
	}

	groupEntries, err := r.refreshGroupCache(containerID, fsys)
	if err != nil {
		return "", err
	}

	groupName, found := groupEntries[gid]
	if !found {
		return "", errGroupNotFound
	}

	return groupName, nil
}

// OnCGroupDeletedEvent is used to handle a CGroupDeleted event
func (r *Resolver) OnCGroupDeletedEvent(sbom *cgroupModel.CacheEntry) {
	r.nsGroupCache.Remove(sbom.ID)
	r.nsUserCache.Remove(sbom.ID)
}

// NewResolver instantiates a new user and group resolver
func NewResolver(cgroupResolver *cgroup.Resolver) (*Resolver, error) {
	nsUserCache, err := lru.New[string, *EntryCache](64)
	if err != nil {
		return nil, err
	}

	nsGroupCache, err := lru.New[string, *EntryCache](64)
	if err != nil {
		return nil, err
	}

	return &Resolver{
		cgroupResolver: cgroupResolver,
		nsUserCache:    nsUserCache,
		nsGroupCache:   nsGroupCache,
	}, nil
}
