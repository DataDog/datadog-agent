// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package uptane

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"github.com/DataDog/go-tuf/client"
	"github.com/DataDog/go-tuf/data"

	"github.com/DataDog/datadog-agent/pkg/config/remote/meta"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// memoryTargetStore serves target file contents from memory. It is used by the
// DC switch recovery to verify a fresh-join payload without writing anything
// to the DB.
type memoryTargetStore struct {
	files map[string][]byte
}

func (s *memoryTargetStore) getTargetFile(path string) ([]byte, bool, error) {
	// go-tuf requests the file under its hash-prefixed path (consistent
	// snapshots): normalize it the same way the target store does.
	file, found := s.files[trimHashTargetPath(path)]
	return file, found, nil
}

// EmbeddedRootVersions returns the versions of the roots embedded in the
// agent binary (the open-source roots shared by every DC). Those are the root
// versions a brand new agent reports on its first request, and the anchor the
// DC switch recovery verifies from.
func (c *CoreAgentClient) EmbeddedRootVersions() (TUFVersions, error) {
	configRoot := meta.RootsConfig(c.site, c.configRootOverride)
	directorRoot := meta.RootsDirector(c.site, c.directorRootOverride)
	return TUFVersions{
		ConfigRoot:   configRoot.Version(),
		DirectorRoot: directorRoot.Version(),
	}, nil
}

// verifiedFreshJoin holds the in-memory verification state of a fresh-join
// payload that was proven valid from the embedded root.
type verifiedFreshJoin struct {
	configTUFClient     *client.Client
	directorTUFClient   *client.Client
	configMemoryStore   client.LocalStore
	directorMemoryStore client.LocalStore
}

// snapshotOrgUUID returns the org UUID signed in the config repository
// snapshot custom of the verified fresh-join payload, if any.
func (v *verifiedFreshJoin) snapshotOrgUUID() (*string, error) {
	metas, err := v.configMemoryStore.GetMeta()
	if err != nil {
		return nil, err
	}
	rawSnapshot, found := metas[metaSnapshot]
	if !found {
		return nil, errors.New("no snapshot metadata in the fresh-join payload")
	}
	rawCustom, err := unsafeMetaCustom(rawSnapshot)
	if err != nil {
		return nil, err
	}
	custom, err := snapshotCustom(rawCustom)
	if err != nil {
		return nil, err
	}
	return custom.OrgUUID, nil
}

// rootRoleKeyIDs returns the key IDs of the root role of a TUF root file:
// the set of keys trusted to sign the next root version, so the identity of
// the chain a root belongs to.
func rootRoleKeyIDs(rootJSON json.RawMessage) (map[string]struct{}, error) {
	var signed data.Signed
	if err := json.Unmarshal(rootJSON, &signed); err != nil {
		return nil, fmt.Errorf("could not parse the root file: %w", err)
	}
	var root data.Root
	if err := json.Unmarshal(signed.Signed, &root); err != nil {
		return nil, fmt.Errorf("could not parse the root file: %w", err)
	}
	role, ok := root.Roles["root"]
	if !ok || len(role.KeyIDs) == 0 {
		return nil, errors.New("the root file does not declare any root role key")
	}
	keyIDs := make(map[string]struct{}, len(role.KeyIDs))
	for _, keyID := range role.KeyIDs {
		keyIDs[keyID] = struct{}{}
	}
	return keyIDs, nil
}

// trustedChainChanged reports whether the root chains of a verified
// fresh-join payload differ from the ones currently latched, for both the
// config and the director repositories. Each DC signs with its own keys, so
// a genuine DC switch always changes both key sets. A payload signed by the
// same keys as the latched state is a replay of the current chain (for
// example a rollback attempt) and must not be accepted through the recovery
// path: see TryRecoverFromEmbeddedRoot.
func (c *CoreAgentClient) trustedChainChanged(verified *verifiedFreshJoin) (bool, error) {
	latchedConfigMetas, err := c.configLocalStore.GetMeta()
	if err != nil {
		return false, fmt.Errorf("could not read the latched config root: %w", err)
	}
	verifiedConfigMetas, err := verified.configMemoryStore.GetMeta()
	if err != nil {
		return false, fmt.Errorf("could not read the verified config root: %w", err)
	}
	latchedConfigKeyIDs, err := rootRoleKeyIDs(latchedConfigMetas[metaRoot])
	if err != nil {
		return false, fmt.Errorf("could not parse the latched config root: %w", err)
	}
	verifiedConfigKeyIDs, err := rootRoleKeyIDs(verifiedConfigMetas[metaRoot])
	if err != nil {
		return false, fmt.Errorf("could not parse the verified config root: %w", err)
	}

	latchedDirectorMetas, err := c.directorLocalStore.GetMeta()
	if err != nil {
		return false, fmt.Errorf("could not read the latched director root: %w", err)
	}
	verifiedDirectorMetas, err := verified.directorMemoryStore.GetMeta()
	if err != nil {
		return false, fmt.Errorf("could not read the verified director root: %w", err)
	}
	latchedDirectorKeyIDs, err := rootRoleKeyIDs(latchedDirectorMetas[metaRoot])
	if err != nil {
		return false, fmt.Errorf("could not parse the latched director root: %w", err)
	}
	verifiedDirectorKeyIDs, err := rootRoleKeyIDs(verifiedDirectorMetas[metaRoot])
	if err != nil {
		return false, fmt.Errorf("could not parse the verified director root: %w", err)
	}

	if maps.Equal(latchedConfigKeyIDs, verifiedConfigKeyIDs) {
		return false, nil
	}
	if maps.Equal(latchedDirectorKeyIDs, verifiedDirectorKeyIDs) {
		return false, nil
	}
	return true, nil
}

// verifyFromEmbeddedRoot verifies a fresh-join response starting from the
// embedded root, in memory only: no DB state is read (beyond the embedded roots
// themselves) or written. It is the exact verification a brand new agent runs on
// its first update: the root chain is walked from the embedded root (each
// rotation step being signed by the previous root keys), then the timestamp,
// snapshot and targets metadata are verified with the keys of the reached root,
// and the Uptane full-image checks compare the director and config
// repositories.
func (c *CoreAgentClient) verifyFromEmbeddedRoot(response *pbgo.LatestConfigsResponse) (*verifiedFreshJoin, error) {
	if response == nil {
		return nil, errors.New("cannot recover from a nil response")
	}

	memoryTargets := &memoryTargetStore{files: make(map[string][]byte)}
	for _, file := range response.TargetFiles {
		// Same normalization as the target store: consistent snapshots make
		// the backend serve target files under a hash-prefixed path.
		memoryTargets.files[trimHashTargetPath(file.Path)] = file.Raw
	}

	configRemoteStore := newRemoteStore(memoryTargets)
	directorRemoteStore := newRemoteStore(memoryTargets)
	if response.ConfigMetas != nil {
		configRemoteStore.updateConfigMetas(response.ConfigMetas)
	}
	if response.DirectorMetas != nil {
		directorRemoteStore.updateDirectorMetas(response.DirectorMetas)
	}

	configMemoryStore := client.MemoryLocalStore()
	directorMemoryStore := client.MemoryLocalStore()

	configTUFClient := client.NewClient(configMemoryStore, &configRemoteStore)
	directorTUFClient := client.NewClient(directorMemoryStore, &directorRemoteStore)

	if err := configTUFClient.Init(json.RawMessage(meta.RootsConfig(c.site, c.configRootOverride).Root())); err != nil {
		return nil, fmt.Errorf("could not initialize the config repository from the embedded root: %w", err)
	}
	if err := directorTUFClient.Init(json.RawMessage(meta.RootsDirector(c.site, c.directorRootOverride).Root())); err != nil {
		return nil, fmt.Errorf("could not initialize the director repository from the embedded root: %w", err)
	}

	if _, err := directorTUFClient.Update(); err != nil {
		return nil, fmt.Errorf("could not verify the director repository of the fresh-join payload from the embedded root: %w", err)
	}
	if _, err := configTUFClient.Update(); err != nil {
		return nil, fmt.Errorf("could not verify the config repository of the fresh-join payload from the embedded root: %w", err)
	}

	if err := c.verifyUptane(configTUFClient, directorTUFClient); err != nil {
		return nil, fmt.Errorf("uptane verification of the fresh-join payload failed: %w", err)
	}

	return &verifiedFreshJoin{
		configTUFClient:     configTUFClient,
		directorTUFClient:   directorTUFClient,
		configMemoryStore:   configMemoryStore,
		directorMemoryStore: directorMemoryStore,
	}, nil
}

// wipeDatabase wipes all the RC state stored in the bolt DB and rebuilds
// the client internals on the emptied DB (local stores reinitialized with
// the embedded roots, empty remote stores, empty target and org stores).
//
// The DB is wiped in place, by deleting its buckets in a single bolt
// transaction, instead of removing and recreating the file: the wipe cannot
// hit a transient file-removal failure (an antivirus scanner or a backup
// tool holding the file open without sharing deletes), the client never
// ends up with a closed DB, and on failure the bolt transaction rolls back
// so the previous state is intact and the recovery can be retried. The DB
// metadata (agent version, API key, URL) belongs to the same agent and org,
// so it is kept.
func (c *CoreAgentClient) wipeDatabase() error {
	// Drop any pending in-memory change before wiping the DB.
	c.transactionalStore.rollback()

	if err := c.transactionalStore.wipeAllData(); err != nil {
		return fmt.Errorf("could not wipe the RC database: %w", err)
	}

	targetStore := newTargetStore(c.transactionalStore)
	c.configRemoteStore = newRemoteStoreConfig(targetStore)
	c.directorRemoteStore = newRemoteStoreDirector(targetStore)
	c.Client.targetStore = targetStore
	c.Client.orgStore = newOrgStore(c.transactionalStore)

	var err error
	if c.configLocalStore, err = newLocalStoreConfig(c.transactionalStore, c.site, c.configRootOverride); err != nil {
		return err
	}
	if c.directorLocalStore, err = newLocalStoreDirector(c.transactionalStore, c.site, c.directorRootOverride); err != nil {
		return err
	}

	c.configTUFClient = client.NewClient(c.configLocalStore, c.configRemoteStore)
	c.directorTUFClient = client.NewClient(c.directorLocalStore, c.directorRemoteStore)

	return nil
}

// TryRecoverFromEmbeddedRoot implements the DC switch recovery. When
// an org moves to another DC behind the same URL, the normal
// TUF update fails permanently: the latched state belongs to the old DC and
// cannot verify the new DC's metadata, and since both DCs share the embedded
// root version no root files are sent to repair the chain.
//
// This method verifies a fresh-join payload (what a brand new agent receives:
// the full root chain from the embedded root, a complete set of metadata)
// entirely in memory, starting from the embedded root. The embedded root is
// the only key material shared across DCs, so it is the only sound anchor to
// decide the new DC's chain is trustworthy; verifying from it introduces no
// new trust compared to a fresh agent.
//
// On success, and only if the payload belongs to the same org (the org UUID
// signed in the new DC's snapshot must match both the org UUID of the API key
// and the previously stored one), the RC DB is wiped and the client rebuilt on
// the new DC's state.
//
// The wipe is further gated on an actual chain change: the verified payload
// must be signed by different root keys than the latched state (each DC signs
// with its own keys). A payload signed by the same keys is a replay of the
// current chain, for example a rollback attempt that the version checks of
// the normal update path already rejected. The DB wipe resets the version
// counters, so accepting such a payload would disable TUF rollback
// protection; it is rejected without touching the state.
//
// A failure before or during the wipe leaves the current state untouched
// (the wipe itself is a single rolled-back bolt transaction). A failure after
// the wipe (the update replay or the commit) leaves the client on the
// emptied DB, latched at the embedded root like a brand new agent: the old
// DC's state is gone, and the next successful update rebuilds the state
// from scratch.
func (c *CoreAgentClient) TryRecoverFromEmbeddedRoot(response *pbgo.LatestConfigsResponse) error {
	c.Lock()
	defer c.Unlock()
	c.cachedVerify = false

	// in case the commit is successful it is a no-op.
	// the defer is present to be sure a transaction is never left behind: it
	// rolls back the store that is current when the function returns (the
	// store is replaced by the wipe below), not the one at defer time.
	defer func() {
		c.transactionalStore.rollback()
	}()

	// The org the agent belongs to, captured before any wipe.
	storedOrgUUID, err := c.StoredOrgUUID()
	if err != nil {
		return fmt.Errorf("could not obtain the current org UUID: %w", err)
	}

	verified, err := c.verifyFromEmbeddedRoot(response)
	if err != nil {
		return err
	}

	// The new DC's snapshot must carry an org UUID and it must match both the
	// org UUID of the API key and the previously stored one. All three agree,
	// or no wipe.
	snapshotOrgUUID, err := verified.snapshotOrgUUID()
	if err != nil {
		return err
	}
	if snapshotOrgUUID == nil {
		return errors.New("the fresh-join payload does not carry an org UUID in its snapshot, cannot confirm it belongs to the same org")
	}
	backendOrgUUID, err := c.orgUUIDProvider()
	if err != nil {
		return fmt.Errorf("could not obtain the org UUID from the backend: %w", err)
	}
	if *snapshotOrgUUID != backendOrgUUID {
		return fmt.Errorf("the fresh-join payload belongs to another org: snapshot org UUID %s, API key org UUID %s", *snapshotOrgUUID, backendOrgUUID)
	}
	if *snapshotOrgUUID != storedOrgUUID {
		return fmt.Errorf("the fresh-join payload belongs to another org: snapshot org UUID %s, stored org UUID %s", *snapshotOrgUUID, storedOrgUUID)
	}

	// The DB wipe resets every version counter, so it must only happen on an
	// actual DC switch, where the counters legitimately restart (they are
	// per-DC and independent). A payload signed by the same keys as the latched
	// state is not a DC switch: it is a replay of the current chain (for
	// example a rollback attempt), which the version checks of the normal
	// update path already rejected. Accepting it here would disable TUF
	// rollback protection, so only wipe when the trusted chain changed.
	chainChanged, err := c.trustedChainChanged(verified)
	if err != nil {
		return err
	}
	if !chainChanged {
		return errors.New("the fresh-join payload is signed by the same keys as the latched state: not a DC switch, keeping the TUF state and its rollback protection")
	}

	// The payload is proven valid from the embedded root and belongs to the
	// same org: wipe the DB and rebuild the client from scratch.
	if err := c.wipeDatabase(); err != nil {
		return err
	}

	// Replay the fresh-join update on the rebuilt state. It must succeed: the
	// in-memory pipeline proved the payload verifies, this time it is applied
	// to the real DB (walking the root chain from the embedded root again).
	if err := c.update(response); err != nil {
		c.resetTUFState()
		return err
	}

	return c.transactionalStore.commit()
}
