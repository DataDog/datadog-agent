// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

package helm

import "sync"

type releasesStore struct {
	store      map[helmStorage]map[namespacedName]map[revision]*release
	storeMutex sync.Mutex
}

func newReleasesStore() *releasesStore {
	res := &releasesStore{
		store: make(map[helmStorage]map[namespacedName]map[revision]*release),
	}

	for _, storageDriver := range []helmStorage{k8sConfigmaps, k8sSecrets} {
		res.store[storageDriver] = make(map[namespacedName]map[revision]*release)
	}

	return res
}

// add stores a release
func (rs *releasesStore) add(rel *release, storage helmStorage) {
	rs.storeMutex.Lock()
	defer rs.storeMutex.Unlock()

	if rs.store[storage][rel.namespacedName()] == nil {
		rs.store[storage][rel.namespacedName()] = make(map[revision]*release)
	}

	rs.store[storage][rel.namespacedName()][rel.revision()] = rel
}

// get returns the release stored with the given namespacedName, revision and
// storage. Returns nil when it does not exist.
func (rs *releasesStore) get(namespacedName namespacedName, revision revision, storage helmStorage) *release {
	rs.storeMutex.Lock()
	defer rs.storeMutex.Unlock()

	if rs.store[storage][namespacedName] == nil {
		return nil
	}

	return rs.store[storage][namespacedName][revision]
}

// getAll returns all the releases stored for the given helmStorage
func (rs *releasesStore) getAll(storage helmStorage) []*release {
	rs.storeMutex.Lock()
	defer rs.storeMutex.Unlock()

	var res []*release

	for _, releasesByRevision := range rs.store[storage] {
		for _, rel := range releasesByRevision {
			res = append(res, rel)
		}
	}

	return res
}

// delete removes a release. It returns a bool that indicates whether the
// release deleted was the only existing revision.
func (rs *releasesStore) delete(rel *release, storage helmStorage) bool {
	rs.storeMutex.Lock()
	defer rs.storeMutex.Unlock()

	if rs.store[storage][rel.namespacedName()] == nil {
		return true
	}

	delete(rs.store[storage][rel.namespacedName()], rel.revision())

	if len(rs.store[storage][rel.namespacedName()]) == 0 {
		delete(rs.store[storage], rel.namespacedName())
		return true
	}

	return false
}
