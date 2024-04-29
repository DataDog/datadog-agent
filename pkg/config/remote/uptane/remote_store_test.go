// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package uptane

import (
	"fmt"
	"io"
	"testing"

	"github.com/DataDog/go-tuf/client"
	"github.com/stretchr/testify/assert"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func generateUpdate(baseVersion uint64) *pbgo.LatestConfigsResponse {
	baseVersion *= 10000
	return &pbgo.LatestConfigsResponse{
		ConfigMetas: &pbgo.ConfigMetas{
			Roots: []*pbgo.TopMeta{
				{
					Raw:     []byte(fmt.Sprintf("root_content_%d", baseVersion+1)),
					Version: baseVersion + 1,
				},
				{
					Raw:     []byte(fmt.Sprintf("root_content_%d", baseVersion+2)),
					Version: baseVersion + 2,
				},
			},
			Timestamp: &pbgo.TopMeta{
				Raw:     []byte(fmt.Sprintf("timestamp_content_%d", baseVersion+3)),
				Version: baseVersion + 3,
			},
			Snapshot: &pbgo.TopMeta{
				Raw:     []byte(fmt.Sprintf("snapshot_content_%d", baseVersion+4)),
				Version: baseVersion + 4,
			},
			TopTargets: &pbgo.TopMeta{
				Raw:     []byte(fmt.Sprintf("targets_content_%d", baseVersion+5)),
				Version: baseVersion + 5,
			},
			DelegatedTargets: []*pbgo.DelegatedMeta{
				{
					Role:    "PRODUCT1",
					Raw:     []byte(fmt.Sprintf("product1_content_%d", baseVersion+6)),
					Version: baseVersion + 6,
				},
				{
					Role:    "PRODUCT2",
					Raw:     []byte(fmt.Sprintf("product1_content_%d", baseVersion+7)),
					Version: baseVersion + 7,
				},
			},
		},
		DirectorMetas: &pbgo.DirectorMetas{
			Roots: []*pbgo.TopMeta{
				{
					Raw:     []byte(fmt.Sprintf("root_content_%d", baseVersion+8)),
					Version: baseVersion + 8,
				},
				{
					Raw:     []byte(fmt.Sprintf("root_content_%d", baseVersion+9)),
					Version: baseVersion + 9,
				},
			},
			Timestamp: &pbgo.TopMeta{
				Raw:     []byte(fmt.Sprintf("timestamp_content_%d", baseVersion+10)),
				Version: baseVersion + 10,
			},
			Snapshot: &pbgo.TopMeta{
				Raw:     []byte(fmt.Sprintf("snapshot_content_%d", baseVersion+11)),
				Version: baseVersion + 11,
			},
			Targets: &pbgo.TopMeta{
				Raw:     []byte(fmt.Sprintf("targets_content_%d", baseVersion+12)),
				Version: baseVersion + 12,
			},
		},
		TargetFiles: []*pbgo.File{
			{
				Raw:  []byte(fmt.Sprintf("config_content_%d", baseVersion)),
				Path: fmt.Sprintf("2/PRODUCT1/6fd7a9e2-3893-4c41-b995-21d41836bc91/config/%d", baseVersion),
			},
			{
				Raw:  []byte(fmt.Sprintf("config_content_%d", baseVersion+1)),
				Path: fmt.Sprintf("2/PRODUCT2/ff7ae782-e418-44e4-95af-47ba3e6bfbf9/config/%d", baseVersion+1),
			},
		},
	}
}

func TestRemoteStoreConfig(t *testing.T) {
	db := newTransactionalStore(getTestDB(t))
	defer db.commit()

	targetStore := newTargetStore(db)
	store := newRemoteStoreConfig(targetStore)

	testUpdate1 := generateUpdate(1)
	targetStore.storeTargetFiles(testUpdate1.TargetFiles)
	store.update(testUpdate1)

	// Checking that timestamp is the only role allowed to perform version-less retrivals
	assertGetMeta(t, &store.remoteStore, "timestamp.json", testUpdate1.ConfigMetas.Timestamp.Raw, nil)
	assertGetMeta(t, &store.remoteStore, "root.json", nil, client.ErrNotFound{File: "root.json"})
	assertGetMeta(t, &store.remoteStore, "targets.json", nil, client.ErrNotFound{File: "targets.json"})
	assertGetMeta(t, &store.remoteStore, "snapshot.json", nil, client.ErrNotFound{File: "snapshot.json"})

	// Checking state matches update1
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.timestamp.json", testUpdate1.ConfigMetas.Timestamp.Version), testUpdate1.ConfigMetas.Timestamp.Raw, nil)
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.snapshot.json", testUpdate1.ConfigMetas.Snapshot.Version), testUpdate1.ConfigMetas.Snapshot.Raw, nil)
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.targets.json", testUpdate1.ConfigMetas.TopTargets.Version), testUpdate1.ConfigMetas.TopTargets.Raw, nil)
	for _, root := range testUpdate1.ConfigMetas.Roots {
		assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.root.json", root.Version), root.Raw, nil)
	}
	for _, delegatedTarget := range testUpdate1.ConfigMetas.DelegatedTargets {
		assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.%s.json", delegatedTarget.Version, delegatedTarget.Role), delegatedTarget.Raw, nil)
	}
	for _, target := range testUpdate1.TargetFiles {
		assertGetTarget(t, &store.remoteStore, target.Path, target.Raw, nil)
	}

	testUpdate2 := generateUpdate(2)
	targetStore.storeTargetFiles(testUpdate2.TargetFiles)
	store.update(testUpdate2)

	// Checking that update1 metas got properly evicted
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.timestamp.json", testUpdate1.ConfigMetas.Timestamp.Version), nil, client.ErrNotFound{File: fmt.Sprintf("%d.timestamp.json", testUpdate1.ConfigMetas.Timestamp.Version)})
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.snapshot.json", testUpdate1.ConfigMetas.Snapshot.Version), nil, client.ErrNotFound{File: fmt.Sprintf("%d.snapshot.json", testUpdate1.ConfigMetas.Snapshot.Version)})
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.targets.json", testUpdate1.ConfigMetas.TopTargets.Version), nil, client.ErrNotFound{File: fmt.Sprintf("%d.targets.json", testUpdate1.ConfigMetas.TopTargets.Version)})
	for _, delegatedTarget := range testUpdate1.ConfigMetas.DelegatedTargets {
		assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.%s.json", delegatedTarget.Version, delegatedTarget.Role), nil, client.ErrNotFound{File: fmt.Sprintf("%d.%s.json", delegatedTarget.Version, delegatedTarget.Role)})
	}
	for _, target := range testUpdate1.TargetFiles {
		assertGetTarget(t, &store.remoteStore, target.Path, target.Raw, nil)
	}

	// Checking that update1 roots got retained
	for _, root := range testUpdate1.ConfigMetas.Roots {
		assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.root.json", root.Version), root.Raw, nil)
	}

	// Checking state matches update2
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.timestamp.json", testUpdate2.ConfigMetas.Timestamp.Version), testUpdate2.ConfigMetas.Timestamp.Raw, nil)
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.snapshot.json", testUpdate2.ConfigMetas.Snapshot.Version), testUpdate2.ConfigMetas.Snapshot.Raw, nil)
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.targets.json", testUpdate2.ConfigMetas.TopTargets.Version), testUpdate2.ConfigMetas.TopTargets.Raw, nil)
	for _, root := range testUpdate2.ConfigMetas.Roots {
		assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.root.json", root.Version), root.Raw, nil)
	}
	for _, delegatedTarget := range testUpdate2.ConfigMetas.DelegatedTargets {
		assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.%s.json", delegatedTarget.Version, delegatedTarget.Role), delegatedTarget.Raw, nil)
	}
	for _, target := range testUpdate2.TargetFiles {
		assertGetTarget(t, &store.remoteStore, target.Path, target.Raw, nil)
	}
}

func TestRemoteStoreDirector(t *testing.T) {
	db := newTransactionalStore(getTestDB(t))
	defer db.commit()
	targetStore := newTargetStore(db)
	store := newRemoteStoreDirector(targetStore)

	testUpdate1 := generateUpdate(1)
	targetStore.storeTargetFiles(testUpdate1.TargetFiles)
	store.update(testUpdate1)

	// Checking that timestamp is the only role allowed to perform version-less retrivals
	assertGetMeta(t, &store.remoteStore, "timestamp.json", testUpdate1.DirectorMetas.Timestamp.Raw, nil)
	assertGetMeta(t, &store.remoteStore, "root.json", nil, client.ErrNotFound{File: "root.json"})
	assertGetMeta(t, &store.remoteStore, "targets.json", nil, client.ErrNotFound{File: "targets.json"})
	assertGetMeta(t, &store.remoteStore, "snapshot.json", nil, client.ErrNotFound{File: "snapshot.json"})

	// Checking state matches update1
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.timestamp.json", testUpdate1.DirectorMetas.Timestamp.Version), testUpdate1.DirectorMetas.Timestamp.Raw, nil)
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.snapshot.json", testUpdate1.DirectorMetas.Snapshot.Version), testUpdate1.DirectorMetas.Snapshot.Raw, nil)
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.targets.json", testUpdate1.DirectorMetas.Targets.Version), testUpdate1.DirectorMetas.Targets.Raw, nil)
	for _, root := range testUpdate1.DirectorMetas.Roots {
		assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.root.json", root.Version), root.Raw, nil)
	}
	for _, target := range testUpdate1.TargetFiles {
		assertGetTarget(t, &store.remoteStore, target.Path, target.Raw, nil)
	}

	testUpdate2 := generateUpdate(2)
	targetStore.storeTargetFiles(testUpdate2.TargetFiles)
	store.update(testUpdate2)

	// Checking that update1 metas got properly evicted
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.timestamp.json", testUpdate1.DirectorMetas.Timestamp.Version), nil, client.ErrNotFound{File: fmt.Sprintf("%d.timestamp.json", testUpdate1.DirectorMetas.Timestamp.Version)})
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.snapshot.json", testUpdate1.DirectorMetas.Snapshot.Version), nil, client.ErrNotFound{File: fmt.Sprintf("%d.snapshot.json", testUpdate1.DirectorMetas.Snapshot.Version)})
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.targets.json", testUpdate1.DirectorMetas.Targets.Version), nil, client.ErrNotFound{File: fmt.Sprintf("%d.targets.json", testUpdate1.DirectorMetas.Targets.Version)})
	for _, target := range testUpdate1.TargetFiles {
		assertGetTarget(t, &store.remoteStore, target.Path, target.Raw, nil)
	}

	// Checking that update1 roots got retained
	for _, root := range testUpdate1.DirectorMetas.Roots {
		assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.root.json", root.Version), root.Raw, nil)
	}

	// Checking state matches update2
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.timestamp.json", testUpdate2.DirectorMetas.Timestamp.Version), testUpdate2.DirectorMetas.Timestamp.Raw, nil)
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.snapshot.json", testUpdate2.DirectorMetas.Snapshot.Version), testUpdate2.DirectorMetas.Snapshot.Raw, nil)
	assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.targets.json", testUpdate2.DirectorMetas.Targets.Version), testUpdate2.DirectorMetas.Targets.Raw, nil)
	for _, root := range testUpdate2.DirectorMetas.Roots {
		assertGetMeta(t, &store.remoteStore, fmt.Sprintf("%d.root.json", root.Version), root.Raw, nil)
	}
	for _, target := range testUpdate2.TargetFiles {
		assertGetTarget(t, &store.remoteStore, target.Path, target.Raw, nil)
	}
}

func assertGetMeta(t *testing.T, store *remoteStore, path string, expectedContent []byte, expectedError error) {
	stream, size, err := store.GetMeta(path)
	if expectedError != nil {
		assert.Equal(t, expectedError, err)
		return
	}
	assert.NoError(t, err)
	assert.Equal(t, int64(len(expectedContent)), size)
	content, err := io.ReadAll(stream)
	assert.NoError(t, err)
	assert.Equal(t, expectedContent, content)
}

func assertGetTarget(t *testing.T, store *remoteStore, path string, expectedContent []byte, expectedError error) {
	stream, size, err := store.GetTarget(path)
	if expectedError != nil {
		assert.Equal(t, expectedError, err)
		return
	}
	assert.NoError(t, err)
	assert.Equal(t, int64(len(expectedContent)), size)
	content, err := io.ReadAll(stream)
	assert.NoError(t, err)
	assert.Equal(t, expectedContent, content)
}
