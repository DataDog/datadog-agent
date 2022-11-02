// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

package helm

import (
	"fmt"
	"sync"
)

type taggedRelease struct {
	release                 *release
	commonTags              []string // tags for service checks, metrics, and events
	tagsForMetricsAndEvents []string // tags only for metrics and events
}

type releasesStore struct {
	store           map[helmStorage]map[namespacedName]map[revision]*taggedRelease
	latestRevisions map[helmStorage]map[namespacedName]revision
	mutex           sync.Mutex
}

func newReleasesStore() *releasesStore {
	res := &releasesStore{
		store:           make(map[helmStorage]map[namespacedName]map[revision]*taggedRelease),
		latestRevisions: make(map[helmStorage]map[namespacedName]revision),
	}

	for _, storageDriver := range []helmStorage{k8sConfigmaps, k8sSecrets} {
		res.store[storageDriver] = make(map[namespacedName]map[revision]*taggedRelease)
		res.latestRevisions[storageDriver] = make(map[namespacedName]revision)
	}

	return res
}

// add stores a release
func (rs *releasesStore) add(rel *release, storage helmStorage, commonTags []string, tagsForMetricsAndEvents []string) (tr *taggedRelease) {
	rs.mutex.Lock()
	defer rs.mutex.Unlock()

	tr = &taggedRelease{
		release:                 rel,
		commonTags:              commonTags,
		tagsForMetricsAndEvents: tagsForMetricsAndEvents,
	}

	if rs.store[storage][rel.namespacedName()] == nil {
		rs.store[storage][rel.namespacedName()] = make(map[revision]*taggedRelease)
	}

	rs.store[storage][rel.namespacedName()][rel.revision()] = tr

	if rel.revision() > rs.latestRevisions[storage][rel.namespacedName()] {
		rs.latestRevisions[storage][rel.namespacedName()] = rel.revision()
	}

	return
}

// get returns the release stored with the given namespacedName, revision and
// storage. Returns nil when it does not exist.
func (rs *releasesStore) get(storage helmStorage, namespacedName namespacedName, revision revision) *taggedRelease {
	rs.mutex.Lock()
	defer rs.mutex.Unlock()

	if rs.store[storage][namespacedName] == nil {
		return nil
	}

	return rs.store[storage][namespacedName][revision]
}

func (rs *releasesStore) getAllTagsForRelease(storage helmStorage, namespacedName namespacedName, revision revision, includeRevision bool) (tags []string) {
	if tr := rs.get(storage, namespacedName, revision); tr != nil {
		tags = append(tr.commonTags, tr.tagsForMetricsAndEvents...)

		if includeRevision {
			tags = append(tags, fmt.Sprintf("helm_revision:%d", revision))
		}

		tags = append(tags, fmt.Sprintf("helm_revision_is_most_recent:%t", revision == rs.latestRevision(storage, namespacedName)))
	}

	return
}

// getAll returns all the releases stored for the given helmStorage
func (rs *releasesStore) getAll(storage helmStorage) []*taggedRelease {
	rs.mutex.Lock()
	defer rs.mutex.Unlock()

	var res []*taggedRelease

	for _, releasesByRevision := range rs.store[storage] {
		for _, rel := range releasesByRevision {
			res = append(res, rel)
		}
	}

	return res
}

// getLatestRevisions returns the releases with the latest revision for the
// given helmStorage
func (rs *releasesStore) getLatestRevisions(storage helmStorage) []*taggedRelease {
	rs.mutex.Lock()
	defer rs.mutex.Unlock()

	var res []*taggedRelease

	for namespaced, rev := range rs.latestRevisions[storage] {
		res = append(res, rs.store[storage][namespaced][rev])
	}

	return res
}

// delete removes a release. It returns a bool that indicates whether there are
// any revisions left for the release.
func (rs *releasesStore) delete(rel *release, storage helmStorage) bool {
	rs.mutex.Lock()
	defer rs.mutex.Unlock()

	if rs.store[storage][rel.namespacedName()] == nil {
		return false
	}

	delete(rs.store[storage][rel.namespacedName()], rel.revision())

	if len(rs.store[storage][rel.namespacedName()]) > 0 {
		rs.mutex.Unlock()
		latestRevision := rs.latestRevision(storage, rel.namespacedName())
		rs.mutex.Lock()

		rs.latestRevisions[storage][rel.namespacedName()] = latestRevision
		return true
	}

	delete(rs.store[storage], rel.namespacedName())
	delete(rs.latestRevisions[storage], rel.namespacedName())

	return false
}

func (rs *releasesStore) latestRevision(storage helmStorage, releaseNamespacedName namespacedName) revision {
	rs.mutex.Lock()
	defer rs.mutex.Unlock()

	latest := revision(0)

	for rev := range rs.store[storage][releaseNamespacedName] {
		if rev > latest {
			latest = rev
		}
	}

	return latest
}
