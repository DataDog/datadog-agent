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

	passwd "github.com/chainguard-dev/go-apk/pkg/passwd"
	lru "github.com/hashicorp/golang-lru/v2"
)

var errUserNotFound = errors.New("user not found")
var errGroupNotFound = errors.New("group not found")

// UserCache defines the cache for uid to usernames
type UserCache map[int]string

// GroupCache defines the cache for gid to group names
type GroupCache map[int]string

// Resolver resolves user and group ids to names
type Resolver struct {
	cgroupResolver *cgroup.Resolver
	nsUserCache    *lru.Cache[string, UserCache]
	nsGroupCache   *lru.Cache[string, GroupCache]
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
func (r *Resolver) RefreshCache(containerID string) (UserCache, GroupCache, error) {
	fsys, err := r.getFilesystem(containerID)
	if err != nil {
		return nil, nil, err
	}

	userCache, err := r.refreshUserCache(containerID, fsys)
	if err != nil {
		return nil, nil, err
	}

	groupCache, err := r.refreshGroupCache(containerID, fsys)
	if err != nil {
		return nil, nil, err
	}

	return userCache, groupCache, nil
}

func (r *Resolver) refreshUserCache(containerID string, fsys fs.FS) (UserCache, error) {
	userFile, err := passwd.ReadUserFile(fsys, "/etc/passwd")
	if err != nil {
		return nil, err
	}

	entryMap := make(map[int]string, len(userFile.Entries))
	for _, entry := range userFile.Entries {
		entryMap[int(entry.UID)] = entry.UserName
	}
	r.nsUserCache.Add(containerID, entryMap)

	return entryMap, nil
}

func (r *Resolver) refreshGroupCache(containerID string, fsys fs.FS) (GroupCache, error) {
	groupFile, err := passwd.ReadGroupFile(fsys, "/etc/group")
	if err != nil {
		return nil, err
	}

	entryMap := make(map[int]string, len(groupFile.Entries))
	for _, entry := range groupFile.Entries {
		entryMap[int(entry.GID)] = entry.GroupName
	}
	r.nsGroupCache.Add(containerID, entryMap)

	return entryMap, nil
}

// ResolveUser resolves a user id to a username
func (r *Resolver) ResolveUser(uid int, containerID string) (string, error) {
	userCache, found := r.nsUserCache.Get(containerID)
	if found {
		cachedEntry, found := userCache[uid]
		if !found {
			return "", errUserNotFound
		}
		return cachedEntry, nil
	}

	fsys, err := r.getFilesystem(containerID)
	if err != nil {
		return "", err
	}

	userCache, err = r.refreshUserCache(containerID, fsys)
	if err != nil {
		return "", err
	}

	userName, found := userCache[uid]
	if !found {
		return "", errUserNotFound
	}

	return userName, nil
}

// ResolveGroup resolves a group id to a group name
func (r *Resolver) ResolveGroup(gid int, containerID string) (string, error) {
	groupCache, found := r.nsGroupCache.Get(containerID)
	if found {
		cachedEntry, found := groupCache[gid]
		if !found {
			return "", errGroupNotFound
		}
		return cachedEntry, nil
	}

	fsys, err := r.getFilesystem(containerID)
	if err != nil {
		return "", err
	}

	groupCache, err = r.refreshGroupCache(containerID, fsys)
	if err != nil {
		return "", err
	}

	groupName, found := groupCache[gid]
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
	nsUserCache, err := lru.New[string, UserCache](64)
	if err != nil {
		return nil, err
	}

	nsGroupCache, err := lru.New[string, GroupCache](64)
	if err != nil {
		return nil, err
	}

	return &Resolver{
		cgroupResolver: cgroupResolver,
		nsUserCache:    nsUserCache,
		nsGroupCache:   nsGroupCache,
	}, nil
}
