// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package uptane

import (
	"bytes"
	"io"

	"github.com/DataDog/go-tuf/client"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

type role string

const (
	roleRoot      role = "root"
	roleTargets   role = "targets"
	roleSnapshot  role = "snapshot"
	roleTimestamp role = "timestamp"
)

// remoteStore implements go-tuf's RemoteStore
// Its goal is to serve TUF metadata updates coming to the backend in a way go-tuf understands
// See https://pkg.go.dev/github.com/DataDog/go-tuf/client#RemoteStore
type remoteStore struct {
	targetStore targetFileProvider
	metas       map[role]map[uint64][]byte
}

// targetFileProvider provides the raw content of a target file.
// It is implemented by the on-disk targetStore and by in-memory
// implementations used to verify updates without touching the DB.
type targetFileProvider interface {
	getTargetFile(path string) ([]byte, bool, error)
}

func newRemoteStore(targetStore targetFileProvider) remoteStore {
	return remoteStore{
		metas: map[role]map[uint64][]byte{
			roleRoot:      make(map[uint64][]byte),
			roleTargets:   make(map[uint64][]byte),
			roleSnapshot:  make(map[uint64][]byte),
			roleTimestamp: make(map[uint64][]byte),
		},
		targetStore: targetStore,
	}
}

func (s *remoteStore) resetRole(r role) {
	s.metas[r] = make(map[uint64][]byte)
}

func (s *remoteStore) latestVersion(r role) uint64 {
	latestVersion := uint64(0)
	for v := range s.metas[r] {
		if v > latestVersion {
			latestVersion = v
		}
	}
	return latestVersion
}

// GetMeta implements go-tuf's RemoteStore.GetMeta
// See https://pkg.go.dev/github.com/DataDog/go-tuf/client#RemoteStore
func (s *remoteStore) GetMeta(path string) (io.ReadCloser, int64, error) {
	metaPath, err := parseMetaPath(path)
	if err != nil {
		return nil, 0, err
	}
	roleVersions, roleFound := s.metas[metaPath.role]
	if !roleFound {
		return nil, 0, client.ErrNotFound{File: path}
	}
	version := metaPath.version
	if !metaPath.versionSet {
		if metaPath.role != roleTimestamp {
			return nil, 0, client.ErrNotFound{File: path}
		}
		version = s.latestVersion(metaPath.role)
	}
	requestedVersion, versionFound := roleVersions[version]
	if !versionFound {
		return nil, 0, client.ErrNotFound{File: path}
	}
	return io.NopCloser(bytes.NewReader(requestedVersion)), int64(len(requestedVersion)), nil
}

// GetTarget implements go-tuf's RemoteStore.GetTarget
// See https://pkg.go.dev/github.com/DataDog/go-tuf/client#RemoteStore
func (s *remoteStore) GetTarget(targetPath string) (stream io.ReadCloser, size int64, err error) {
	target, found, err := s.targetStore.getTargetFile(targetPath)
	if err != nil {
		return nil, 0, err
	}
	if !found {
		return nil, 0, client.ErrNotFound{File: targetPath}
	}
	return io.NopCloser(bytes.NewReader(target)), int64(len(target)), nil
}

type remoteStoreDirector struct {
	remoteStore
}

func newRemoteStoreDirector(targetStore *targetStore) *remoteStoreDirector {
	return &remoteStoreDirector{remoteStore: newRemoteStore(targetStore)}
}

func (sd *remoteStoreDirector) update(update *pbgo.LatestConfigsResponse) {
	if update == nil {
		return
	}
	if update.DirectorMetas == nil {
		return
	}
	sd.updateDirectorMetas(update.DirectorMetas)
}

func (s *remoteStore) updateDirectorMetas(metas *pbgo.DirectorMetas) {
	for _, root := range metas.Roots {
		s.metas[roleRoot][root.Version] = root.Raw
	}
	if metas.Timestamp != nil {
		s.resetRole(roleTimestamp)
		s.metas[roleTimestamp][metas.Timestamp.Version] = metas.Timestamp.Raw
	}
	if metas.Snapshot != nil {
		s.resetRole(roleSnapshot)
		s.metas[roleSnapshot][metas.Snapshot.Version] = metas.Snapshot.Raw
	}
	if metas.Targets != nil {
		s.resetRole(roleTargets)
		s.metas[roleTargets][metas.Targets.Version] = metas.Targets.Raw
	}
}

type remoteStoreConfig struct {
	remoteStore
}

func newRemoteStoreConfig(targetStore *targetStore) *remoteStoreConfig {
	return &remoteStoreConfig{
		remoteStore: newRemoteStore(targetStore),
	}
}

func (sc *remoteStoreConfig) update(update *pbgo.LatestConfigsResponse) {
	if update == nil {
		return
	}
	if update.ConfigMetas == nil {
		return
	}
	sc.updateConfigMetas(update.ConfigMetas)
}

func (s *remoteStore) updateConfigMetas(metas *pbgo.ConfigMetas) {
	for _, root := range metas.Roots {
		s.metas[roleRoot][root.Version] = root.Raw
	}
	for _, delegatedMeta := range metas.DelegatedTargets {
		role := role(delegatedMeta.Role)
		s.resetRole(role)
		s.metas[role][delegatedMeta.Version] = delegatedMeta.Raw
	}
	if metas.Timestamp != nil {
		s.resetRole(roleTimestamp)
		s.metas[roleTimestamp][metas.Timestamp.Version] = metas.Timestamp.Raw
	}
	if metas.Snapshot != nil {
		s.resetRole(roleSnapshot)
		s.metas[roleSnapshot][metas.Snapshot.Version] = metas.Snapshot.Raw
	}
	if metas.TopTargets != nil {
		s.resetRole(roleTargets)
		s.metas[roleTargets][metas.TopTargets.Version] = metas.TopTargets.Raw
	}
}
