// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package uptane

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.etcd.io/bbolt"

	"github.com/DataDog/datadog-agent/pkg/config/remote/meta"
)

func getTestDB(t *testing.T) *bbolt.DB {
	dir := t.TempDir()
	db, err := bbolt.Open(dir+"/remote-config.db", 0600, &bbolt.Options{})
	if err != nil {
		panic(err)
	}
	t.Cleanup(func() {
		db.Close()
	})
	return db
}

func TestLocalStore(t *testing.T) {
	db := getTestDB(t)
	embededRoots := map[uint64]meta.EmbeddedRoot{
		1: []byte(`{"signatures":[{"keyid":"44d70fa8eae4c07f26c2767270827b6b9e11e7972926b3b419b5ea14ec32f796","sig":"366534e35c3ac0749d5b60f12ab32da736863315bb4765eeb7b24417e8b8c40aace37649a12c63f8ad3634fbe2e68711655e72120934cc015414c75725861e08"},{"keyid":"b2b93a6dccc96d053e6db39181124c85ba4156d43503d4351b5500316fa084e8","sig":"ada4a7723d462eb4c1f087025f81f5eab5de48cb18b710de94ad2194ee9e0524fafe6eaddf95e894808f8254380a86f8f7219d69bf693d6e1c80db904a47830e"}],"signed":{"_type":"root","consistent_snapshot":true,"expires":"1970-01-01T00:00:00Z","keys":{"44d70fa8eae4c07f26c2767270827b6b9e11e7972926b3b419b5ea14ec32f796":{"keyid_hash_algorithms":["sha256","sha512"],"keytype":"ed25519","keyval":{"public":"286d6ae328365afec0f92519ceab68cd627e34072cde90b2f5d167badea970f2"},"scheme":"ed25519"},"b2b93a6dccc96d053e6db39181124c85ba4156d43503d4351b5500316fa084e8":{"keyid_hash_algorithms":["sha256","sha512"],"keytype":"ed25519","keyval":{"public":"afdd68be53815d67f8fa99cf101aac4589a358c660adf7dd4e179fe96834d3c9"},"scheme":"ed25519"}},"roles":{"root":{"keyids":["44d70fa8eae4c07f26c2767270827b6b9e11e7972926b3b419b5ea14ec32f796","b2b93a6dccc96d053e6db39181124c85ba4156d43503d4351b5500316fa084e8"],"threshold":2},"snapshot":{"keyids":["44d70fa8eae4c07f26c2767270827b6b9e11e7972926b3b419b5ea14ec32f796","b2b93a6dccc96d053e6db39181124c85ba4156d43503d4351b5500316fa084e8"],"threshold":2},"targets":{"keyids":["44d70fa8eae4c07f26c2767270827b6b9e11e7972926b3b419b5ea14ec32f796","b2b93a6dccc96d053e6db39181124c85ba4156d43503d4351b5500316fa084e8"],"threshold":2},"timestamp":{"keyids":["44d70fa8eae4c07f26c2767270827b6b9e11e7972926b3b419b5ea14ec32f796","b2b93a6dccc96d053e6db39181124c85ba4156d43503d4351b5500316fa084e8"],"threshold":2}},"spec_version":"1.0","version":1}}`),
		2: []byte(`{"signatures":[{"keyid":"key","sig":"sig2"},{"keyid":"b2b93a6dccc96d053e6db39181124c85ba4156d43503d4351b5500316fa084e8","sig":"ada4a7723d462eb4c1f087025f81f5eab5de48cb18b710de94ad2194ee9e0524fafe6eaddf95e894808f8254380a86f8f7219d69bf693d6e1c80db904a47830e"}],"signed":{"_type":"root","consistent_snapshot":true,"expires":"1970-01-01T00:00:00Z","keys":{"44d70fa8eae4c07f26c2767270827b6b9e11e7972926b3b419b5ea14ec32f796":{"keyid_hash_algorithms":["sha256","sha512"],"keytype":"ed25519","keyval":{"public":"286d6ae328365afec0f92519ceab68cd627e34072cde90b2f5d167badea970f2"},"scheme":"ed25519"},"b2b93a6dccc96d053e6db39181124c85ba4156d43503d4351b5500316fa084e8":{"keyid_hash_algorithms":["sha256","sha512"],"keytype":"ed25519","keyval":{"public":"afdd68be53815d67f8fa99cf101aac4589a358c660adf7dd4e179fe96834d3c9"},"scheme":"ed25519"}},"roles":{"root":{"keyids":["44d70fa8eae4c07f26c2767270827b6b9e11e7972926b3b419b5ea14ec32f796","b2b93a6dccc96d053e6db39181124c85ba4156d43503d4351b5500316fa084e8"],"threshold":2},"snapshot":{"keyids":["44d70fa8eae4c07f26c2767270827b6b9e11e7972926b3b419b5ea14ec32f796","b2b93a6dccc96d053e6db39181124c85ba4156d43503d4351b5500316fa084e8"],"threshold":2},"targets":{"keyids":["44d70fa8eae4c07f26c2767270827b6b9e11e7972926b3b419b5ea14ec32f796","b2b93a6dccc96d053e6db39181124c85ba4156d43503d4351b5500316fa084e8"],"threshold":2},"timestamp":{"keyids":["44d70fa8eae4c07f26c2767270827b6b9e11e7972926b3b419b5ea14ec32f796","b2b93a6dccc96d053e6db39181124c85ba4156d43503d4351b5500316fa084e8"],"threshold":2}},"spec_version":"1.0","version":2}}`),
	}
	transactionalStore := newTransactionalStore(db)

	store, err := newLocalStore(transactionalStore, "test", embededRoots)
	assert.NoError(t, err)
	storeRoot1 := json.RawMessage(embededRoots[1])
	storeRoot2 := json.RawMessage(embededRoots[2])

	rootVersion, err := store.GetMetaVersion("root.json")
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), rootVersion)

	metas, err := store.GetMeta()
	assert.NoError(t, err)
	assert.Equal(t, map[string]json.RawMessage{
		"root.json": storeRoot2,
	}, metas)

	storeTimestamp5 := json.RawMessage(`{"signatures":[],"signed":{"_type":"timestamp","version":5}}`)
	err = store.SetMeta("timestamp.json", storeTimestamp5)
	assert.NoError(t, err)
	metas, err = store.GetMeta()
	assert.NoError(t, err)
	assert.Equal(t, map[string]json.RawMessage{
		"root.json":      storeRoot2,
		"timestamp.json": storeTimestamp5,
	}, metas)

	storeSnapshot6 := json.RawMessage(`{"signatures":[],"signed":{"_type":"snapshot","version":6}}`)
	err = store.SetMeta("snapshot.json", storeSnapshot6)
	assert.NoError(t, err)
	metas, err = store.GetMeta()
	assert.NoError(t, err)
	assert.Equal(t, map[string]json.RawMessage{
		"root.json":      storeRoot2,
		"timestamp.json": storeTimestamp5,
		"snapshot.json":  storeSnapshot6,
	}, metas)

	storeTargets7 := json.RawMessage(`{"signatures":[],"signed":{"_type":"targets","version":7}}`)
	err = store.SetMeta("targets.json", storeTargets7)
	assert.NoError(t, err)
	metas, err = store.GetMeta()
	assert.NoError(t, err)
	assert.Equal(t, map[string]json.RawMessage{
		"root.json":      storeRoot2,
		"timestamp.json": storeTimestamp5,
		"snapshot.json":  storeSnapshot6,
		"targets.json":   storeTargets7,
	}, metas)

	storeRoot3 := `{"signatures":[{"keyid":"44d70fa8eae4c07f26c2767270827b6b9e11e7972926b3b419b5ea14ec32f796","sig":"366534e35c3ac0749d5b60f12ab32da736863315bb4765eeb7b24417e8b8c40aace37649a12c63f8ad3634fbe2e68711655e72120934cc015414c75725861e08"},{"keyid":"b2b93a6dccc96d053e6db39181124c85ba4156d43503d4351b5500316fa084e8","sig":"ada4a7723d462eb4c1f087025f81f5eab5de48cb18b710de94ad2194ee9e0524fafe6eaddf95e894808f8254380a86f8f7219d69bf693d6e1c80db904a47830e"}],"signed":{"_type":"root","consistent_snapshot":true,"expires":"1970-01-01T00:00:00Z","keys":{"44d70fa8eae4c07f26c2767270827b6b9e11e7972926b3b419b5ea14ec32f796":{"keyid_hash_algorithms":["sha256","sha512"],"keytype":"ed25519","keyval":{"public":"286d6ae328365afec0f92519ceab68cd627e34072cde90b2f5d167badea970f2"},"scheme":"ed25519"},"b2b93a6dccc96d053e6db39181124c85ba4156d43503d4351b5500316fa084e8":{"keyid_hash_algorithms":["sha256","sha512"],"keytype":"ed25519","keyval":{"public":"afdd68be53815d67f8fa99cf101aac4589a358c660adf7dd4e179fe96834d3c9"},"scheme":"ed25519"}},"roles":{"root":{"keyids":["44d70fa8eae4c07f26c2767270827b6b9e11e7972926b3b419b5ea14ec32f796","b2b93a6dccc96d053e6db39181124c85ba4156d43503d4351b5500316fa084e8"],"threshold":2},"snapshot":{"keyids":["44d70fa8eae4c07f26c2767270827b6b9e11e7972926b3b419b5ea14ec32f796","b2b93a6dccc96d053e6db39181124c85ba4156d43503d4351b5500316fa084e8"],"threshold":2},"targets":{"keyids":["44d70fa8eae4c07f26c2767270827b6b9e11e7972926b3b419b5ea14ec32f796","b2b93a6dccc96d053e6db39181124c85ba4156d43503d4351b5500316fa084e8"],"threshold":2},"timestamp":{"keyids":["44d70fa8eae4c07f26c2767270827b6b9e11e7972926b3b419b5ea14ec32f796","b2b93a6dccc96d053e6db39181124c85ba4156d43503d4351b5500316fa084e8"],"threshold":2}},"spec_version":"1.0","version":3}}`
	err = store.SetMeta("root.json", json.RawMessage(storeRoot3))
	assert.NoError(t, err)

	rootVersion, err = store.GetMetaVersion("root.json")
	assert.NoError(t, err)
	assert.Equal(t, uint64(3), rootVersion)

	metas, err = store.GetMeta()
	assert.NoError(t, err)
	assert.Equal(t, map[string]json.RawMessage{
		"root.json":      json.RawMessage(storeRoot3),
		"timestamp.json": storeTimestamp5,
		"snapshot.json":  storeSnapshot6,
		"targets.json":   storeTargets7,
	}, metas)

	err = store.DeleteMeta("timestamp.json")
	assert.NoError(t, err)
	metas, err = store.GetMeta()
	assert.NoError(t, err)
	assert.Equal(t, map[string]json.RawMessage{
		"root.json":     json.RawMessage(storeRoot3),
		"snapshot.json": storeSnapshot6,
		"targets.json":  storeTargets7,
	}, metas)

	root1, found, err := store.GetRoot(1)
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []byte(storeRoot1), root1)

	root2, found, err := store.GetRoot(2)
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []byte(storeRoot2), root2)

	root3, found, err := store.GetRoot(3)
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []byte(storeRoot3), root3)

	_, found, err = store.GetRoot(4)
	assert.NoError(t, err)
	assert.False(t, found)
}
