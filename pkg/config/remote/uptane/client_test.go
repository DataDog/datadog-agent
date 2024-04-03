// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package uptane

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/go-tuf/data"
	"github.com/DataDog/go-tuf/pkg/keys"
	"github.com/DataDog/go-tuf/sign"
	"github.com/stretchr/testify/assert"
	"go.etcd.io/bbolt"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/meta"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func getTestOrgUUIDProvider(orgID int) OrgUUIDProvider {
	return func() (string, error) {
		return getTestOrgUUIDFromID(orgID), nil
	}
}

func getTestOrgUUIDFromID(orgID int) string {
	return fmt.Sprintf("org-%d-uuid", orgID)
}

func newTestConfig(repo testRepositories) model.Config {
	cfg := model.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	cfg.SetWithoutSource("remote_configuration.director_root", repo.directorRoot)
	cfg.SetWithoutSource("remote_configuration.config_root", repo.configRoot)
	return cfg
}

func newTestClient(db *bbolt.DB, cfg model.Config) (*Client, error) {
	opts := []ClientOption{
		WithOrgIDCheck(2),
		WithConfigRootOverride("datadoghq.com", cfg.GetString("remote_configuration.config_root")),
		WithDirectorRootOverride("datadoghq.com", cfg.GetString("remote_configuration.director_root")),
	}
	return NewClient(db, getTestOrgUUIDProvider(2), opts...)
}

func TestClientState(t *testing.T) {
	testRepository1 := newTestRepository(2, 1, nil, nil, nil)
	cfg := newTestConfig(testRepository1)
	db := getTestDB(t)
	client1, err := newTestClient(db, cfg)
	assert.NoError(t, err)

	// Testing default state
	clientState, err := client1.State()
	assert.NoError(t, err)
	assert.Equal(t, meta.RootsConfig("datadoghq.com", cfg.GetString("remote_configuration.config_root")).LastVersion(), clientState.ConfigRootVersion())
	assert.Equal(t, meta.RootsDirector("datadoghq.com", cfg.GetString("remote_configuration.director_root")).LastVersion(), clientState.DirectorRootVersion())
	_, err = client1.TargetsMeta()
	assert.Error(t, err)

	// Testing state for a simple valid repository
	err = client1.Update(testRepository1.toUpdate())
	assert.NoError(t, err)
	clientState, err = client1.State()
	assert.NoError(t, err)
	assert.Equal(t, uint64(testRepository1.configSnapshotVersion), clientState.ConfigSnapshotVersion())
	assert.Equal(t, uint64(testRepository1.configRootVersion), clientState.ConfigRootVersion())
	assert.Equal(t, uint64(testRepository1.directorRootVersion), clientState.DirectorRootVersion())
	assert.Equal(t, uint64(testRepository1.directorTargetsVersion), clientState.DirectorTargetsVersion())
	targets1, err := client1.TargetsMeta()
	assert.NoError(t, err)
	assert.Equal(t, string(testRepository1.directorTargets), string(targets1))

	// Testing state is maintained between runs
	client2, err := newTestClient(db, cfg)
	assert.NoError(t, err)
	clientState, err = client2.State()
	assert.NoError(t, err)
	assert.Equal(t, uint64(testRepository1.configSnapshotVersion), clientState.ConfigSnapshotVersion())
	assert.Equal(t, uint64(testRepository1.configRootVersion), clientState.ConfigRootVersion())
	assert.Equal(t, uint64(testRepository1.directorRootVersion), clientState.DirectorRootVersion())
	assert.Equal(t, uint64(testRepository1.directorTargetsVersion), clientState.DirectorTargetsVersion())
	targets1, err = client2.TargetsMeta()
	assert.NoError(t, err)
	assert.Equal(t, string(testRepository1.directorTargets), string(targets1))
}

func TestClientFullState(t *testing.T) {
	target1content, target1 := generateTarget()
	_, target2 := generateTarget()
	configTargets := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/1": target1,
		"datadog/2/APM_SAMPLING/id/2": target2,
	}
	directorTargets := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/1": target1,
	}
	testRepository := newTestRepository(2, 1, configTargets, directorTargets, []*pbgo.File{{Path: "datadog/2/APM_SAMPLING/id/1", Raw: target1content}})
	cfg := newTestConfig(testRepository)
	db := getTestDB(t)

	// Prepare
	client, err := newTestClient(db, cfg)
	assert.NoError(t, err)
	err = client.Update(testRepository.toUpdate())
	assert.NoError(t, err)
	_, err = client.TargetFile("datadog/2/APM_SAMPLING/id/1")
	assert.NoError(t, err)

	// Check full state
	state, err := client.State()
	assert.NoError(t, err)
	assert.Equal(t, 4, len(state.ConfigState))
	assert.Equal(t, 4, len(state.DirectorState))
	assert.Equal(t, 1, len(state.TargetFilenames))

	assertMetaVersion(t, state.ConfigState, "root.json", 1)
	assertMetaVersion(t, state.ConfigState, "timestamp.json", 11)
	assertMetaVersion(t, state.ConfigState, "targets.json", 101)
	assertMetaVersion(t, state.ConfigState, "snapshot.json", 1001)

	assertMetaVersion(t, state.DirectorState, "root.json", 1)
	assertMetaVersion(t, state.DirectorState, "timestamp.json", 21)
	assertMetaVersion(t, state.DirectorState, "targets.json", 201)
	assertMetaVersion(t, state.DirectorState, "snapshot.json", 2001)
}

func assertMetaVersion(t *testing.T, state map[string]MetaState, metaName string, version uint64) {
	metaState, found := state[metaName]
	assert.True(t, found)
	assert.Equal(t, version, metaState.Version)
}

func TestClientVerifyTUF(t *testing.T) {
	testRepository1 := newTestRepository(2, 1, nil, nil, nil)
	cfg := newTestConfig(testRepository1)
	db := getTestDB(t)

	previousConfigTargets := testRepository1.configTargets
	client1, err := newTestClient(db, cfg)
	assert.NoError(t, err)
	testRepository1.configTargets = generateTargets(generateKey(), testRepository1.configTargetsVersion, nil)
	err = client1.Update(testRepository1.toUpdate())
	assert.Error(t, err)

	testRepository1.configTargets = previousConfigTargets
	client2, err := newTestClient(db, cfg)
	assert.NoError(t, err)
	testRepository1.directorTargets = generateTargets(generateKey(), testRepository1.directorTargetsVersion, nil)
	err = client2.Update(testRepository1.toUpdate())
	assert.Error(t, err)
}

func TestClientVerifyUptane(t *testing.T) {
	target1content, target1 := generateTarget()
	target2content, target2 := generateTarget()
	configTargets1 := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/1": target1,
		"datadog/2/APM_SAMPLING/id/2": target2,
	}
	directorTargets1 := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/1": target1,
	}
	configTargets2 := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/1": target1,
	}
	directorTargets2 := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/1": target1,
		"datadog/2/APM_SAMPLING/id/2": target2,
	}
	testRepositoryValid := newTestRepository(2, 1, configTargets1, directorTargets1, []*pbgo.File{{Path: "datadog/2/APM_SAMPLING/id/1", Raw: target1content}})
	testRepositoryInvalid1 := newTestRepository(2, 1, configTargets2, directorTargets2, []*pbgo.File{{Path: "datadog/2/APM_SAMPLING/id/1", Raw: target1content}, {Path: "datadog/2/APM_SAMPLING/id/2", Raw: target2content}})

	cfgValid := newTestConfig(testRepositoryValid)
	db := getTestDB(t)

	client1, err := newTestClient(db, cfgValid)
	assert.NoError(t, err)
	err = client1.Update(testRepositoryValid.toUpdate())
	assert.NoError(t, err)
	targetFile, err := client1.TargetFile("datadog/2/APM_SAMPLING/id/1")
	assert.NoError(t, err)
	assert.Equal(t, target1content, targetFile)

	cfgInvalid1 := newTestConfig(testRepositoryInvalid1)
	client2, err := newTestClient(db, cfgInvalid1)
	assert.NoError(t, err)
	err = client2.Update(testRepositoryInvalid1.toUpdate())
	assert.Error(t, err)
	_, err = client1.TargetFile("datadog/2/APM_SAMPLING/id/2")
	assert.Error(t, err)
}

func TestClientVerifyOrgID(t *testing.T) {
	db := getTestDB(t)

	target1content, target1 := generateTarget()
	_, target2 := generateTarget()
	configTargets1 := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/1": target1,
		"datadog/2/APM_SAMPLING/id/2": target2,
	}
	directorTargets1 := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/1": target1,
	}
	configTargets2 := data.TargetFiles{
		"datadog/3/APM_SAMPLING/id/1": target1,
		"datadog/3/APM_SAMPLING/id/2": target2,
	}
	directorTargets2 := data.TargetFiles{
		"datadog/3/APM_SAMPLING/id/1": target1,
	}
	testRepositoryValid := newTestRepository(2, 1, configTargets1, directorTargets1, []*pbgo.File{{Path: "datadog/2/APM_SAMPLING/id/1", Raw: target1content}})
	testRepositoryInvalid := newTestRepository(2, 1, configTargets2, directorTargets2, []*pbgo.File{{Path: "datadog/3/APM_SAMPLING/id/1", Raw: target1content}})

	cfgValid := newTestConfig(testRepositoryValid)

	client1, err := newTestClient(db, cfgValid)
	assert.NoError(t, err)
	err = client1.Update(testRepositoryValid.toUpdate())
	assert.NoError(t, err)

	cfgInvalid := newTestConfig(testRepositoryInvalid)
	client2, err := newTestClient(db, cfgInvalid)
	assert.NoError(t, err)
	err = client2.Update(testRepositoryInvalid.toUpdate())
	assert.Error(t, err)
}

func TestClientVerifyOrgUUID(t *testing.T) {
	db := getTestDB(t)

	target1content, target1 := generateTarget()
	_, target2 := generateTarget()
	configTargets := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/1": target1,
		"datadog/2/APM_SAMPLING/id/2": target2,
	}
	directorTargets := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/1": target1,
	}

	testRepositoryValid := newTestRepository(2, 1, configTargets, directorTargets, []*pbgo.File{{Path: "datadog/2/APM_SAMPLING/id/1", Raw: target1content}})
	testRepositoryValidNoUUID := newTestRepository(4, 1, configTargets, directorTargets, []*pbgo.File{{Path: "datadog/2/APM_SAMPLING/id/1", Raw: target1content}})
	testRepositoryInvalid := newTestRepository(3, 1, configTargets, directorTargets, []*pbgo.File{{Path: "datadog/2/APM_SAMPLING/id/1", Raw: target1content}})

	cfgValid := newTestConfig(testRepositoryValid)
	cfgValidNoUUID := newTestConfig(testRepositoryValidNoUUID)
	cfgInvalid := newTestConfig(testRepositoryInvalid)

	// Valid repository with an orgID and a UUID in the snapshot
	client1, err := newTestClient(db, cfgValid)
	assert.NoError(t, err)
	err = client1.Update(testRepositoryValid.toUpdate())
	assert.NoError(t, err)

	// Valid repository with an orgID but no UUID in the snapshot
	db2 := getTestDB(t)
	client2, err := newTestClient(db2, cfgValidNoUUID)
	assert.NoError(t, err)
	err = client2.Update(testRepositoryValidNoUUID.toUpdate())
	assert.Error(t, err)

	// Invalid repository : receives snapshot with orgUUID for org 2, but is org 3
	client3, err := newTestClient(db, cfgInvalid)
	assert.NoError(t, err)
	err = client3.Update(testRepositoryInvalid.toUpdate())
	assert.Error(t, err)
}

func TestOrgStore(t *testing.T) {
	db := getTestDB(t)
	client, err := NewClient(db, getTestOrgUUIDProvider(2), WithOrgIDCheck(2))
	assert.NoError(t, err)

	// Store key
	err = client.orgStore.storeOrgUUID(0, "abc")
	assert.NoError(t, err)

	// Get stored key
	uuid, exists, err := client.orgStore.getOrgUUID(0)
	assert.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, "abc", uuid)

	// Get non stored key
	uuid, exists, err = client.orgStore.getOrgUUID(1)
	assert.NoError(t, err) // Should not error except when store fails
	assert.False(t, exists)
	assert.Equal(t, "", uuid)

	// Root rotation!
	err = client.orgStore.storeOrgUUID(1, "def")
	assert.NoError(t, err)

	// Possibility to get new UUID
	uuid, exists, err = client.orgStore.getOrgUUID(1)
	assert.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, "def", uuid)
}

func generateKey() keys.Signer {
	key, _ := keys.GenerateEd25519Key()
	return key
}

type testRepositories struct {
	configTimestampKey   keys.Signer
	configTargetsKey     keys.Signer
	configSnapshotKey    keys.Signer
	configRootKey        keys.Signer
	directorTimestampKey keys.Signer
	directorTargetsKey   keys.Signer
	directorSnapshotKey  keys.Signer
	directorRootKey      keys.Signer

	configTimestampVersion   int64
	configTargetsVersion     int64
	configSnapshotVersion    int64
	configRootVersion        int64
	directorTimestampVersion int64
	directorTargetsVersion   int64
	directorSnapshotVersion  int64
	directorRootVersion      int64

	configTimestamp   []byte
	configTargets     []byte
	configSnapshot    []byte
	configRoot        []byte
	directorTimestamp []byte
	directorTargets   []byte
	directorSnapshot  []byte
	directorRoot      []byte

	targetFiles []*pbgo.File
}

func newTestRepository(snapshotOrgID int, version int64, configTargets data.TargetFiles, directorTargets data.TargetFiles, targetFiles []*pbgo.File) testRepositories {
	repos := testRepositories{
		configTimestampKey:   generateKey(),
		configTargetsKey:     generateKey(),
		configSnapshotKey:    generateKey(),
		configRootKey:        generateKey(),
		directorTimestampKey: generateKey(),
		directorTargetsKey:   generateKey(),
		directorSnapshotKey:  generateKey(),
		directorRootKey:      generateKey(),
		targetFiles:          targetFiles,
	}
	repos.configRootVersion = version
	repos.configTimestampVersion = 10 + version
	repos.configTargetsVersion = 100 + version
	repos.configSnapshotVersion = 1000 + version
	repos.directorRootVersion = version
	repos.directorTimestampVersion = 20 + version
	repos.directorTargetsVersion = 200 + version
	repos.directorSnapshotVersion = 2000 + version
	repos.configRoot = generateRoot(repos.configRootKey, version, repos.configTimestampKey, repos.configTargetsKey, repos.configSnapshotKey, nil)
	repos.configTargets = generateTargets(repos.configTargetsKey, 100+version, configTargets)
	repos.configSnapshot = generateSnapshot(snapshotOrgID, repos.configSnapshotKey, 1000+version, repos.configTargetsVersion)
	repos.configTimestamp = generateTimestamp(repos.configTimestampKey, 10+version, repos.configSnapshotVersion, repos.configSnapshot)
	repos.directorRoot = generateRoot(repos.directorRootKey, version, repos.directorTimestampKey, repos.directorTargetsKey, repos.directorSnapshotKey, nil)
	repos.directorTargets = generateTargets(repos.directorTargetsKey, 200+version, directorTargets)
	repos.directorSnapshot = generateSnapshot(snapshotOrgID, repos.directorSnapshotKey, 2000+version, repos.directorTargetsVersion)
	repos.directorTimestamp = generateTimestamp(repos.directorTimestampKey, 20+version, repos.directorSnapshotVersion, repos.directorSnapshot)
	return repos
}

func (r testRepositories) toUpdate() *pbgo.LatestConfigsResponse {
	return &pbgo.LatestConfigsResponse{
		ConfigMetas: &pbgo.ConfigMetas{
			Roots:      []*pbgo.TopMeta{{Version: uint64(r.configRootVersion), Raw: r.configRoot}},
			Timestamp:  &pbgo.TopMeta{Version: uint64(r.configTimestampVersion), Raw: r.configTimestamp},
			Snapshot:   &pbgo.TopMeta{Version: uint64(r.configSnapshotVersion), Raw: r.configSnapshot},
			TopTargets: &pbgo.TopMeta{Version: uint64(r.configTargetsVersion), Raw: r.configTargets},
		},
		DirectorMetas: &pbgo.DirectorMetas{
			Roots:     []*pbgo.TopMeta{{Version: uint64(r.directorRootVersion), Raw: r.directorRoot}},
			Timestamp: &pbgo.TopMeta{Version: uint64(r.directorTimestampVersion), Raw: r.directorTimestamp},
			Snapshot:  &pbgo.TopMeta{Version: uint64(r.directorSnapshotVersion), Raw: r.directorSnapshot},
			Targets:   &pbgo.TopMeta{Version: uint64(r.directorTargetsVersion), Raw: r.directorTargets},
		},
		TargetFiles: r.targetFiles,
	}
}

func generateRoot(key keys.Signer, version int64, timestampKey keys.Signer, targetsKey keys.Signer, snapshotKey keys.Signer, previousRootKey keys.Signer) []byte {
	root := data.NewRoot()
	root.Version = version
	root.Expires = time.Now().Add(1 * time.Hour)
	root.AddKey(key.PublicData())
	root.AddKey(timestampKey.PublicData())
	root.AddKey(targetsKey.PublicData())
	root.AddKey(snapshotKey.PublicData())
	root.Roles["root"] = &data.Role{
		KeyIDs:    key.PublicData().IDs(),
		Threshold: 1,
	}
	root.Roles["timestamp"] = &data.Role{
		KeyIDs:    timestampKey.PublicData().IDs(),
		Threshold: 1,
	}
	root.Roles["targets"] = &data.Role{
		KeyIDs:    targetsKey.PublicData().IDs(),
		Threshold: 1,
	}
	root.Roles["snapshot"] = &data.Role{
		KeyIDs:    snapshotKey.PublicData().IDs(),
		Threshold: 1,
	}

	rootSigners := []keys.Signer{key}
	if previousRootKey != nil {
		rootSigners = append(rootSigners, previousRootKey)
	}

	signedRoot, _ := sign.Marshal(&root, rootSigners...)
	serializedRoot, _ := json.Marshal(signedRoot)
	return serializedRoot
}

func generateTimestamp(key keys.Signer, version int64, snapshotVersion int64, snapshot []byte) []byte {
	meta := data.NewTimestamp()
	meta.Expires = time.Now().Add(1 * time.Hour)
	meta.Version = version
	meta.Meta["snapshot.json"] = data.TimestampFileMeta{Version: snapshotVersion, Length: int64(len(snapshot)), Hashes: data.Hashes{
		"sha256": hashSha256(snapshot),
	}}
	signed, _ := sign.Marshal(&meta, key)
	serialized, _ := json.Marshal(signed)
	return serialized
}

func generateTargets(key keys.Signer, version int64, targets data.TargetFiles) []byte {
	meta := data.NewTargets()
	meta.Expires = time.Now().Add(1 * time.Hour)
	meta.Version = version
	meta.Targets = targets
	signed, _ := sign.Marshal(&meta, key)
	serialized, _ := json.Marshal(signed)
	return serialized
}

func generateSnapshot(orgID int, key keys.Signer, version int64, targetsVersion int64) []byte {
	meta := data.NewSnapshot()
	meta.Expires = time.Now().Add(1 * time.Hour)
	meta.Version = version
	meta.Meta["targets.json"] = data.SnapshotFileMeta{Version: targetsVersion}

	if orgID != 0 {
		uuid := getTestOrgUUIDFromID(orgID)
		customData := &snapshotCustomData{OrgUUID: &uuid}
		customDataBytes, _ := json.Marshal(customData)
		customDataBytesRaw := json.RawMessage(customDataBytes)
		meta.Custom = &customDataBytesRaw
	}

	signed, _ := sign.Marshal(&meta, key)
	serialized, _ := json.Marshal(signed)
	return serialized
}

func hashSha256(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}

func generateTarget() ([]byte, data.TargetFileMeta) {
	file := make([]byte, 128)
	rand.Read(file)
	return file, data.TargetFileMeta{
		FileMeta: data.FileMeta{
			Length: int64(len(file)),
			Hashes: data.Hashes{
				"sha256": hashSha256(file),
			},
		},
	}
}
