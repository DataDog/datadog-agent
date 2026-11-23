// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package uptane

import (
	"testing"

	"github.com/DataDog/go-tuf/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// newDCTestTopology builds the two-DC topology: a shared
// embedded-style root (version 17) and two DCs that each rotated once to root
// version 18 with their own DC-local keys and independent version counters.
// dcA uses low version counters, dcB higher ones, so a switch from A to B is
// not a version regression: the update failure is purely a signature
// verification failure.
func newDCTestTopology(configTargets data.TargetFiles, directorTargets data.TargetFiles, targetFiles []*pbgo.File) (embedded testRepositories, dcA testRepositories, dcB testRepositories) {
	embedded = newTestEmbeddedRepository(2, 17, configTargets, directorTargets, targetFiles)
	dcA = embedded.rotateForDC(2, configTargets, directorTargets, targetFiles, 1)
	dcB = embedded.rotateForDC(2, configTargets, directorTargets, targetFiles, 200)
	return embedded, dcA, dcB
}

// newDCTestClient creates a client that only trusts the embedded root, like
// the agent does out of the box.
func newDCTestClient(t *testing.T, embedded testRepositories) *CoreAgentClient {
	t.Helper()
	cfg := newTestConfig(t, embedded)
	ts := getTestTransactionalStore(t)
	client, err := newTestClient(ts, cfg)
	require.NoError(t, err)
	return client
}

// assertClientLatchedToDC asserts the client is latched to the given DC's
// metadata versions.
func assertClientLatchedToDC(t *testing.T, client *CoreAgentClient, repo testRepositories) {
	t.Helper()
	state, err := client.State()
	require.NoError(t, err)
	assert.Equal(t, uint64(repo.configRootVersion), state.ConfigRootVersion())
	assert.Equal(t, uint64(repo.configSnapshotVersion), state.ConfigSnapshotVersion())
	assert.Equal(t, uint64(repo.directorRootVersion), state.DirectorRootVersion())
	assert.Equal(t, uint64(repo.directorTargetsVersion), state.DirectorTargetsVersion())
}

func dcTestTargets() (data.TargetFiles, data.TargetFiles, []*pbgo.File) {
	target1content, target1 := generateTarget()
	_, target2 := generateTarget()

	configTargets := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/1": target1,
		"datadog/2/APM_SAMPLING/id/2": target2,
	}
	directorTargets := data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/1": target1,
	}
	targetFiles := []*pbgo.File{{Path: "datadog/2/APM_SAMPLING/id/1", Raw: target1content}}
	return configTargets, directorTargets, targetFiles
}

// TestTryRecoverFromEmbeddedRootDCSwitchAccepted covers a DC switch: the
// org moves to another DC behind the same URL. The agent's latched TUF state
// belongs to the old DC, the normal update cannot repair it (no root files
// are sent since both DCs are at root version 18), and the fresh-join payload
// of the new DC must be verified from the embedded root, then latched after a
// DB wipe.
func TestTryRecoverFromEmbeddedRootDCSwitchAccepted(t *testing.T) {
	configTargets, directorTargets, targetFiles := dcTestTargets()
	embedded, dcA, dcB := newDCTestTopology(configTargets, directorTargets, targetFiles)

	client := newDCTestClient(t, embedded)

	// Latch DC A through its rotation from the embedded root.
	err := client.Update(dcA.toUpdate())
	require.NoError(t, err)
	assertClientLatchedToDC(t, client, dcA)

	// The org moves to DC B behind the same URL. Both DCs are at root version
	// 18, so the backend sends no roots and the update fails: B's metadata is
	// signed with keys the latched A chain does not know, even though B's
	// version counters are higher than A's.
	err = client.Update(dcB.toUpdateNoRoots())
	assert.Error(t, err)

	// The fresh-join payload of DC B (what a brand new agent receives) must
	// verify from the embedded root and be accepted after a DB wipe.
	err = client.TryRecoverFromEmbeddedRoot(dcB.toUpdate())
	require.NoError(t, err)

	// The client is now latched to DC B.
	assertClientLatchedToDC(t, client, dcB)

	// The roots chain is fully readable: the embedded root stays available
	// for partial clients, the latched root is B's rotation.
	directorRoot17, err := client.DirectorRoot(17)
	require.NoError(t, err)
	assert.Equal(t, embedded.directorRoot, directorRoot17)
	directorRoot18, err := client.DirectorRoot(18)
	require.NoError(t, err)
	assert.Equal(t, dcB.directorRoot, directorRoot18)

	// DC B's configs were applied.
	target1content, err := client.TargetFile("datadog/2/APM_SAMPLING/id/1")
	require.NoError(t, err)
	assert.Equal(t, targetFiles[0].Raw, target1content)

	// A normal incremental update from DC B keeps working on the rebuilt state.
	dcB2 := dcB.incrementalChange(configTargets, directorTargets, targetFiles, nil, dcB.directorSnapshotVersion+1)
	err = client.Update(dcB2.toUpdate())
	require.NoError(t, err)
	assertClientLatchedToDC(t, client, dcB2)
}

// TestTryRecoverFromEmbeddedRootSameChainReplayRejected covers the rollback
// protection. A replay of older, still unexpired metadata from the current DC
// verifies from the embedded root (the current chain anchors there too) and
// belongs to the same org, but it is signed by the same keys as the latched
// state: it is not a DC switch. The recovery must reject it: wiping the DB
// would reset the version counters and let the rolled back versions in,
// disabling TUF rollback protection.
func TestTryRecoverFromEmbeddedRootSameChainReplayRejected(t *testing.T) {
	configTargets, directorTargets, targetFiles := dcTestTargets()
	embedded, _, dcB := newDCTestTopology(configTargets, directorTargets, targetFiles)

	client := newDCTestClient(t, embedded)

	// Latch DC B, then advance it once: the client is at dcB2's versions.
	err := client.Update(dcB.toUpdate())
	require.NoError(t, err)
	dcB2 := dcB.incrementalChange(configTargets, directorTargets, targetFiles, nil, dcB.directorSnapshotVersion+1)
	err = client.Update(dcB2.toUpdate())
	require.NoError(t, err)
	assertClientLatchedToDC(t, client, dcB2)

	// An older payload of the same DC is a rollback: the normal update
	// rejects it on its version checks, which is what triggers the recovery
	// in the service.
	err = client.Update(dcB.toUpdateNoRoots())
	assert.Error(t, err)

	// The recovery must reject the same-chain payload and leave the latched
	// state untouched.
	err = client.TryRecoverFromEmbeddedRoot(dcB.toUpdate())
	require.Error(t, err)
	assertClientLatchedToDC(t, client, dcB2)

	// The rollback is still rejected by the normal update path afterwards.
	err = client.Update(dcB.toUpdateNoRoots())
	assert.Error(t, err)
	assertClientLatchedToDC(t, client, dcB2)
}

// TestTryRecoverFromEmbeddedRootRequiresFreshJoinResponse asserts the recovery
// cannot succeed from the payload the backend sends after the DC switch: with
// no root files in the response, the new DC's chain is not verifiable from the
// latched state, which is why the service must re-request a fresh-join payload.
func TestTryRecoverFromEmbeddedRootRequiresFreshJoinResponse(t *testing.T) {
	configTargets, directorTargets, targetFiles := dcTestTargets()
	embedded, dcA, dcB := newDCTestTopology(configTargets, directorTargets, targetFiles)

	client := newDCTestClient(t, embedded)
	err := client.Update(dcA.toUpdate())
	require.NoError(t, err)
	assertClientLatchedToDC(t, client, dcA)

	// The normal response (no roots) is not enough to recover.
	err = client.TryRecoverFromEmbeddedRoot(dcB.toUpdateNoRoots())
	assert.Error(t, err)

	// The state was left untouched.
	assertClientLatchedToDC(t, client, dcA)
	directorRoot18, err := client.DirectorRoot(18)
	require.NoError(t, err)
	assert.Equal(t, dcA.directorRoot, directorRoot18)
}

// TestTryRecoverFromEmbeddedRootOrgMismatchRejected asserts the recovery
// refuses to wipe and re-latch when the fresh-join payload belongs to another
// org: the snapshot custom org UUID must match the org UUID of the API key.
func TestTryRecoverFromEmbeddedRootOrgMismatchRejected(t *testing.T) {
	configTargets, directorTargets, targetFiles := dcTestTargets()
	embedded, dcA, _ := newDCTestTopology(configTargets, directorTargets, targetFiles)

	// DC B serves another org behind the same URL.
	dcBOtherOrg := embedded.rotateForDC(3, configTargets, directorTargets, targetFiles, 200)

	client := newDCTestClient(t, embedded)
	err := client.Update(dcA.toUpdate())
	require.NoError(t, err)
	assertClientLatchedToDC(t, client, dcA)

	err = client.TryRecoverFromEmbeddedRoot(dcBOtherOrg.toUpdate())
	assert.Error(t, err)

	// No wipe happened: the DB is still latched to DC A.
	assertClientLatchedToDC(t, client, dcA)
	directorRoot18, err := client.DirectorRoot(18)
	require.NoError(t, err)
	assert.Equal(t, dcA.directorRoot, directorRoot18)
}

// TestTryRecoverFromEmbeddedRootUntrustedChainRejected asserts the recovery
// refuses a root chain that does not verify from the embedded root: DC B's
// root version 18 is not signed by the embedded (shared) root keys, so the
// rotation step is invalid.
func TestTryRecoverFromEmbeddedRootUntrustedChainRejected(t *testing.T) {
	configTargets, directorTargets, targetFiles := dcTestTargets()
	embedded, dcA, dcB := newDCTestTopology(configTargets, directorTargets, targetFiles)

	// DC B's rotation is not cross-signed by the embedded root keys.
	dcBUntrusted := dcB
	dcBUntrusted.configRoot = generateRoot(dcB.configRootKey, dcB.configRootVersion, dcB.configTimestampKey, dcB.configTargetsKey, dcB.configSnapshotKey, nil, nil)
	dcBUntrusted.directorRoot = generateRoot(dcB.directorRootKey, dcB.directorRootVersion, dcB.directorTimestampKey, dcB.directorTargetsKey, dcB.directorSnapshotKey, nil, nil)

	client := newDCTestClient(t, embedded)
	err := client.Update(dcA.toUpdate())
	require.NoError(t, err)
	assertClientLatchedToDC(t, client, dcA)

	err = client.TryRecoverFromEmbeddedRoot(dcBUntrusted.toUpdate())
	assert.Error(t, err)

	// No wipe happened.
	assertClientLatchedToDC(t, client, dcA)
	directorRoot18, err := client.DirectorRoot(18)
	require.NoError(t, err)
	assert.Equal(t, dcA.directorRoot, directorRoot18)
}

// TestTryRecoverFromEmbeddedRootUntrustedPayloadRejected asserts the recovery
// refuses a payload whose metadata is signed with keys outside the new DC's
// verified root chain.
func TestTryRecoverFromEmbeddedRootUntrustedPayloadRejected(t *testing.T) {
	configTargets, directorTargets, targetFiles := dcTestTargets()
	embedded, dcA, dcB := newDCTestTopology(configTargets, directorTargets, targetFiles)

	// DC B's snapshot is signed by a key that is not in B's root chain.
	rogueKey := generateKey()
	dcBRogueSnapshot := dcB
	dcBRogueSnapshot.configSnapshot = generateSnapshot(2, rogueKey, dcB.configSnapshotVersion, dcB.configTargetsVersion, nil)
	dcBRogueSnapshot.configTimestamp = generateTimestamp(dcB.configTimestampKey, dcB.configTimestampVersion, dcB.configSnapshotVersion, dcBRogueSnapshot.configSnapshot, nil)

	client := newDCTestClient(t, embedded)
	err := client.Update(dcA.toUpdate())
	require.NoError(t, err)
	assertClientLatchedToDC(t, client, dcA)

	err = client.TryRecoverFromEmbeddedRoot(dcBRogueSnapshot.toUpdate())
	assert.Error(t, err)

	// No wipe happened.
	assertClientLatchedToDC(t, client, dcA)
	directorRoot18, err := client.DirectorRoot(18)
	require.NoError(t, err)
	assert.Equal(t, dcA.directorRoot, directorRoot18)
}

// TestTryRecoverFromEmbeddedRootWipeKeepsDBFileAndMetadata covers the DB
// wipe itself: the DB is emptied in place, by deleting its buckets, instead
// of removing and recreating the file. After the recovery the DB file is the
// same and its metadata (agent version, API key, URL) is intact, so a
// restart opens the same DB; none of the old DC's data is left behind.
func TestTryRecoverFromEmbeddedRootWipeKeepsDBFileAndMetadata(t *testing.T) {
	configTargets, directorTargets, targetFiles := dcTestTargets()
	embedded, dcA, dcB := newDCTestTopology(configTargets, directorTargets, targetFiles)

	client := newDCTestClient(t, embedded)

	err := client.Update(dcA.toUpdate())
	require.NoError(t, err)
	assertClientLatchedToDC(t, client, dcA)

	dbPath := client.transactionalStore.db.Path()
	metadataBefore, err := getMetadata(client.transactionalStore.db)
	require.NoError(t, err)

	err = client.TryRecoverFromEmbeddedRoot(dcB.toUpdate())
	require.NoError(t, err)
	assertClientLatchedToDC(t, client, dcB)

	// The DB file was kept, not deleted and recreated.
	assert.Equal(t, dbPath, client.transactionalStore.db.Path())

	// The DB metadata is intact, so a restart opens the same DB.
	metadataAfter, err := getMetadata(client.transactionalStore.db)
	require.NoError(t, err)
	assert.Equal(t, metadataBefore, metadataAfter)

	// None of the old DC's data is left: the latched root is the new DC's
	// rotation, not the old one.
	directorRoot18, err := client.DirectorRoot(18)
	require.NoError(t, err)
	assert.Equal(t, dcB.directorRoot, directorRoot18)
}
