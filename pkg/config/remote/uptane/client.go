// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package uptane contains the logic needed to perform the Uptane verification
// checks against stored TUF metadata and the associated config files.
package uptane

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/go-tuf/client"
	"github.com/DataDog/go-tuf/data"

	rdata "github.com/DataDog/datadog-agent/pkg/config/remote/data"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// Client is an uptane client
type Client struct {
	sync.Mutex

	site            string
	orgID           int64
	orgUUIDProvider OrgUUIDProvider

	configLocalStore *localStore
	configTUFClient  *client.Client

	configRootOverride string
	directorLocalStore *localStore
	directorTUFClient  *client.Client

	directorRootOverride string
	targetStore          *targetStore
	orgStore             *orgStore

	cachedVerify     bool
	cachedVerifyTime time.Time

	// cachedDirectorTargets and cachedTargetFiles hold the verified read-state
	// produced by the most recent successful verifyUptane() pass. They let
	// reads (Targets/TargetFile/TargetFiles) avoid re-downloading and
	// re-verifying target files that were already verified moments earlier.
	// They are only valid when cachedVerify is true; they must be rebuilt
	// (or invalidated) any time verifyUptane runs or the client is updated.
	cachedDirectorTargets data.TargetFiles
	cachedTargetFiles     map[string][]byte

	// TUF transaction tracker
	transactionalStore *transactionalStore

	orgVerificationEnabled bool
}

// CoreAgentClient is an uptane client that fetches the latest configs from the Core Agent
type CoreAgentClient struct {
	*Client
	configRemoteStore   *remoteStoreConfig
	directorRemoteStore *remoteStoreDirector
}

// ClientOption describes a function in charge of changing the uptane client
type ClientOption func(c *Client)

// WithOrgIDCheck sets the org ID
func WithOrgIDCheck(orgID int64) ClientOption {
	return func(c *Client) {
		c.orgID = orgID
	}
}

// WithDirectorRootOverride overrides director root
func WithDirectorRootOverride(site string, directorRootOverride string) ClientOption {
	return func(c *Client) {
		c.site = site
		c.directorRootOverride = directorRootOverride
	}
}

// WithConfigRootOverride overrides config root
func WithConfigRootOverride(site string, configRootOverride string) ClientOption {
	return func(c *Client) {
		c.site = site
		c.configRootOverride = configRootOverride
	}
}

// OrgUUIDProvider is a provider of the agent org UUID
type OrgUUIDProvider func() (string, error)

// newCoreAgentClient creates a new uptane client
func newCoreAgentClient(transactionalStore *transactionalStore, orgUUIDProvider OrgUUIDProvider, options ...ClientOption) (c *CoreAgentClient, err error) {
	targetStore := newTargetStore(transactionalStore)
	orgStore := newOrgStore(transactionalStore)

	c = &CoreAgentClient{
		configRemoteStore:   newRemoteStoreConfig(targetStore),
		directorRemoteStore: newRemoteStoreDirector(targetStore),
		Client: &Client{
			orgStore:               orgStore,
			orgUUIDProvider:        orgUUIDProvider,
			targetStore:            targetStore,
			transactionalStore:     transactionalStore,
			orgVerificationEnabled: true,
		},
	}

	for _, o := range options {
		o(c.Client)
	}

	if c.configLocalStore, err = newLocalStoreConfig(transactionalStore, c.site, c.configRootOverride); err != nil {
		return nil, err
	}

	if c.directorLocalStore, err = newLocalStoreDirector(transactionalStore, c.site, c.directorRootOverride); err != nil {
		return nil, err
	}

	c.configTUFClient = client.NewClient(c.configLocalStore, c.configRemoteStore)
	c.directorTUFClient = client.NewClient(c.directorLocalStore, c.directorRemoteStore)
	return c, nil
}

// NewCoreAgentClientWithNewTransactionalStore creates a new uptane client with a new transactional store
func NewCoreAgentClientWithNewTransactionalStore(dbMetadata *Metadata, orgUUIDProvider OrgUUIDProvider, options ...ClientOption) (c *CoreAgentClient, err error) {
	transactionalStore, err := newTransactionalStore(dbMetadata)
	if err != nil {
		return nil, err
	}

	return newCoreAgentClient(transactionalStore, orgUUIDProvider, options...)
}

// NewCoreAgentClientWithRecreatedTransactionalStore creates a new uptane client with a recreated transactional store
func NewCoreAgentClientWithRecreatedTransactionalStore(dbMetadata *Metadata, orgUUIDProvider OrgUUIDProvider, options ...ClientOption) (c *CoreAgentClient, err error) {
	transactionalStore, err := recreateTransactionalStore(dbMetadata)
	if err != nil {
		return nil, err
	}

	return newCoreAgentClient(transactionalStore, orgUUIDProvider, options...)
}

// Update updates the uptane client and rollbacks in case of error
func (c *CoreAgentClient) Update(response *pbgo.LatestConfigsResponse) error {
	c.Lock()
	defer c.Unlock()
	c.cachedVerify = false
	c.invalidateReadCache()

	// in case the commit is successful it is a no-op.
	// the defer is present to be sure a transaction is never left behind.
	defer c.transactionalStore.rollback()

	err := c.update(response)
	if err != nil {
		c.configRemoteStore = newRemoteStoreConfig(c.targetStore)
		c.directorRemoteStore = newRemoteStoreDirector(c.targetStore)
		c.configTUFClient = client.NewClient(c.configLocalStore, c.configRemoteStore)
		c.directorTUFClient = client.NewClient(c.directorLocalStore, c.directorRemoteStore)
		// Rebuilt TUF clients invalidate any previously cached read-state a
		// second time, in case verifyUptane had somehow already re-populated
		// it before the failure was detected.
		c.invalidateReadCache()
		return err
	}
	return c.transactionalStore.commit()
}

// invalidateReadCache clears the cached verified read-state (director
// targets and target file contents). Must be called with the Client's lock
// held. It must run on every Update() call, on both the success and failure
// path, so the cache can never be served against TUF clients that have since
// been rebuilt.
func (c *Client) invalidateReadCache() {
	c.cachedDirectorTargets = nil
	c.cachedTargetFiles = nil
}

// update updates the uptane client
func (c *CoreAgentClient) update(response *pbgo.LatestConfigsResponse) error {
	err := c.updateRepos(response)
	if err != nil {
		return err
	}
	err = c.pruneTargetFiles()
	if err != nil {
		return err
	}
	return c.verify()
}

func (c *CoreAgentClient) updateRepos(response *pbgo.LatestConfigsResponse) error {
	err := c.targetStore.storeTargetFiles(response.TargetFiles)
	if err != nil {
		return err
	}
	c.directorRemoteStore.update(response)
	c.configRemoteStore.update(response)
	_, err = c.directorTUFClient.Update()
	if err != nil {
		return fmt.Errorf("failed updating director repository: %w", err)
	}
	_, err = c.configTUFClient.Update()
	if err != nil {
		return fmt.Errorf("could not update config repository [%s]: %w", configMetasUpdateSummary(response.ConfigMetas), err)
	}
	return nil
}

// Close will close the underlying boltDB
func (c *Client) Close() error {
	return c.transactionalStore.Close()
}

// TargetsCustom returns the current targets custom of this uptane client
func (c *Client) TargetsCustom() ([]byte, error) {
	c.Lock()
	defer c.Unlock()
	return c.directorLocalStore.GetMetaCustom(metaTargets)
}

// TimestampExpires returns the expiry time of the current up-to-date timestamp.json
func (c *Client) TimestampExpires() (time.Time, error) {
	c.Lock()
	defer c.Unlock()
	return c.directorLocalStore.GetMetaExpires(metaTimestamp)
}

// DirectorRoot returns a director root
func (c *Client) DirectorRoot(version uint64) ([]byte, error) {
	c.Lock()
	defer c.Unlock()
	err := c.verify()
	if err != nil {
		return nil, err
	}
	root, found, err := c.directorLocalStore.GetRoot(version)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("director root version %d was not found in local store", version)
	}
	return root, nil
}

func (c *Client) unsafeTargets() (data.TargetFiles, error) {
	err := c.verify()
	if err != nil {
		return nil, err
	}
	// cachedDirectorTargets is only nil when the read cache has not been
	// (re)populated since the last invalidation (e.g. an Update() call).
	// verify() only reports success after verifyUptane has run and
	// populated the cache (even to an empty-but-non-nil map when there are
	// no director targets), so this should always hit in the common case;
	// falling back to a direct fetch here is just a defensive cache-miss path.
	if c.cachedDirectorTargets != nil {
		return c.cachedDirectorTargets, nil
	}
	return c.directorTUFClient.Targets()
}

// Targets returns the current targets of this uptane client
func (c *Client) Targets() (data.TargetFiles, error) {
	c.Lock()
	defer c.Unlock()
	return c.unsafeTargets()
}

func (c *Client) unsafeTargetFile(path string) ([]byte, error) {
	err := c.verify()
	if err != nil {
		return nil, err
	}
	if content, found := c.cachedTargetFiles[path]; found {
		return content, nil
	}
	// Cache miss (e.g. the path isn't part of the currently verified target
	// set, or the cache hasn't been populated yet): fall back to the direct
	// download exactly as before.
	buffer := &bufferDestination{}
	err = c.directorTUFClient.Download(path, buffer)
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// TargetFile returns the content of a target if the repository is in a verified state
func (c *Client) TargetFile(path string) ([]byte, error) {
	c.Lock()
	defer c.Unlock()
	return c.unsafeTargetFile(path)
}

// TargetFiles returns the content of various multiple target files if the repository is in a
// verified state.
func (c *Client) TargetFiles(targetFiles []string) (map[string][]byte, error) {
	c.Lock()
	defer c.Unlock()

	err := c.verify()
	if err != nil {
		return nil, err
	}

	files := make(map[string][]byte, len(targetFiles))
	var missing []string
	for _, path := range targetFiles {
		if content, found := c.cachedTargetFiles[path]; found {
			files[path] = content
			continue
		}
		missing = append(missing, path)
	}
	if len(missing) == 0 {
		return files, nil
	}

	// Build the storage space for the cache-miss paths only.
	destinations := make(map[string]client.Destination, len(missing))
	for _, path := range missing {
		destinations[path] = &bufferDestination{}
	}

	err = c.directorTUFClient.DownloadBatch(destinations)
	if err != nil {
		return nil, err
	}

	// Build the return type
	for path, contents := range destinations {
		files[path] = contents.(*bufferDestination).Bytes()
	}

	return files, nil
}

func (c *Client) unsafeTargetsMeta() ([]byte, error) {
	metas, err := c.directorLocalStore.GetMeta()
	if err != nil {
		return nil, err
	}
	targets, found := metas[metaTargets]
	if !found {
		return nil, errors.New("empty targets meta in director local store")
	}
	return targets, nil
}

// TargetsMeta verifies and returns the current raw targets.json meta of this uptane client
func (c *Client) TargetsMeta() ([]byte, error) {
	c.Lock()
	defer c.Unlock()
	err := c.verify()
	if err != nil {
		return nil, err
	}
	return c.unsafeTargetsMeta()
}

// UnsafeTargetsMeta returns the current raw targets.json meta of this uptane client without verifying
func (c *Client) UnsafeTargetsMeta() ([]byte, error) {
	c.Lock()
	defer c.Unlock()
	return c.unsafeTargetsMeta()
}

func (c *Client) pruneTargetFiles() error {
	targetFiles, err := c.directorTUFClient.Targets()
	if err != nil {
		return err
	}
	var keptTargetFiles []string
	for target := range targetFiles {
		keptTargetFiles = append(keptTargetFiles, target)
	}
	return c.targetStore.pruneTargetFiles(keptTargetFiles)
}

func (c *Client) verify() error {
	if c.cachedVerify && time.Since(c.cachedVerifyTime) < time.Minute {
		return nil
	}
	err := c.verifyOrg()
	if err != nil {
		return err
	}
	err = c.verifyUptane()
	if err != nil {
		return err
	}
	c.cachedVerify = true
	c.cachedVerifyTime = time.Now()
	return nil
}

// StoredOrgUUID returns the org UUID given by the backend
func (c *Client) StoredOrgUUID() (string, error) {
	// This is an important block of code : to avoid being locked out
	// of the agent in case of a wrong uuid being stored, we link an
	// org UUID storage to a root version. What this means in practice
	// is that if we ever get locked out due to a problem in the orgUUID
	// storage, we can issue a root rotation to unlock ourselves.
	rootVersion, err := c.configLocalStore.GetMetaVersion(metaRoot)
	if err != nil {
		return "", err
	}
	orgUUID, found, err := c.orgStore.getOrgUUID(rootVersion)
	if err != nil {
		return "", err
	}
	if !found {
		orgUUID, err = c.orgUUIDProvider()
		if err != nil {
			return "", err
		}
		err := c.orgStore.storeOrgUUID(rootVersion, orgUUID)
		if err != nil {
			return "", fmt.Errorf("could not store orgUUID in the org store: %v", err)
		}
	}
	return orgUUID, nil
}

func (c *Client) verifyOrg() error {
	if !c.orgVerificationEnabled {
		return nil
	}
	rawCustom, err := c.configLocalStore.GetMetaCustom(metaSnapshot)
	if err != nil {
		return fmt.Errorf("could not obtain snapshot custom: %v", err)
	}
	custom, err := snapshotCustom(rawCustom)
	if err != nil {
		return fmt.Errorf("could not parse snapshot custom: %v", err)
	}
	// Another safeguard here: if we ever get locked out of agents,
	// we can remove the orgUUID from the snapshot and they'll work
	// again. This being said, this is last resort.
	if custom.OrgUUID != nil {
		orgUUID, err := c.StoredOrgUUID()
		if err != nil {
			return fmt.Errorf("could not obtain stored/remote orgUUID: %v", err)
		}
		if *custom.OrgUUID != orgUUID {
			return fmt.Errorf("stored/remote OrgUUID and snapshot OrgUUID do not match: stored=%s received=%s", orgUUID, *custom.OrgUUID)
		}
	}
	// skip the orgID check when no orgID was provided to the client
	if c.orgID == 0 {
		return nil
	}
	directorTargets, err := c.directorTUFClient.Targets()
	if err != nil {
		return err
	}
	for targetPath := range directorTargets {
		configPathMeta, err := rdata.ParseConfigPath(targetPath)
		if err != nil {
			return err
		}
		checkOrgID := configPathMeta.Source != rdata.SourceEmployee
		if checkOrgID && configPathMeta.OrgID != c.orgID {
			return fmt.Errorf(
				"director target '%s' does not have the correct orgID. %d != %d",
				targetPath, configPathMeta.OrgID, c.orgID,
			)
		}
	}
	return nil
}

func (c *Client) verifyUptane() error {
	directorTargets, err := c.directorTUFClient.Targets()
	if err != nil {
		return err
	}
	if len(directorTargets) == 0 {
		// Nothing to verify. Reset the read cache to a populated-but-empty
		// state rather than leaving it untouched: leaving it untouched could
		// mean serving a half-populated or stale cache left over from a
		// previous verifyUptane call (e.g. targets that have since been
		// removed).
		c.cachedDirectorTargets = data.TargetFiles{}
		c.cachedTargetFiles = map[string][]byte{}
		return nil
	}

	// NOTE: the config-repo and director-repo downloads below must use
	// separate destination maps. bufferDestination embeds bytes.Buffer,
	// whose Write appends rather than overwrites; reusing one map for both
	// DownloadBatch calls would silently double the cached content (the
	// second repo's bytes get appended onto the first repo's bytes for the
	// same path). The two repos' downloads are only used to cross-validate
	// hash/length below and, now, to populate the read cache from the
	// director-repo copy specifically -- the same copy Targets()/
	// TargetFile()/TargetFiles() serve today via the director TUF client.
	configPathsDestinations := make(map[string]client.Destination, len(directorTargets))
	directorPathsDestinations := make(map[string]client.Destination, len(directorTargets))
	targetPaths := make([]string, 0, len(directorTargets))
	for targetPath := range directorTargets {
		targetPaths = append(targetPaths, targetPath)
		configPathsDestinations[targetPath] = &bufferDestination{}
		directorPathsDestinations[targetPath] = &bufferDestination{}
	}
	configTargetMetas, err := c.configTUFClient.TargetBatch(targetPaths)
	if err != nil {
		if client.IsNotFound(err) {
			return fmt.Errorf("failed to find target in config repository: %w", err)
		}
		// Other errors such as expired metadata
		return err
	}

	for targetPath, targetMeta := range directorTargets {
		configTargetMeta := configTargetMetas[targetPath]
		if configTargetMeta.Length != targetMeta.Length {
			return fmt.Errorf("target '%s' has size %d in directory repository and %d in config repository", targetPath, configTargetMeta.Length, targetMeta.Length)
		}
		if len(targetMeta.Hashes) == 0 {
			return fmt.Errorf("target '%s' no hashes in the director repository", targetPath)
		}
		if len(targetMeta.Hashes) != len(configTargetMeta.Hashes) {
			return fmt.Errorf("target '%s' has %d hashes in directory repository and %d hashes in config repository", targetPath, len(targetMeta.Hashes), len(configTargetMeta.Hashes))
		}
		for hashAlgo, directorHash := range targetMeta.Hashes {
			configHash, found := configTargetMeta.Hashes[hashAlgo]
			if !found {
				return fmt.Errorf("hash '%s' found in directory repository but not in the config repository", directorHash)
			}
			if !bytes.Equal([]byte(directorHash), []byte(configHash)) {
				return fmt.Errorf("directory hash '%s' does not match config repository '%s'", string(directorHash), string(configHash))
			}
		}
	}
	// Check that the files are valid in the context of the TUF repository (path in targets, hash matching)
	err = c.configTUFClient.DownloadBatch(configPathsDestinations)
	if err != nil {
		return err
	}
	err = c.directorTUFClient.DownloadBatch(directorPathsDestinations)
	if err != nil {
		return err
	}

	// Both repos' downloads succeeded and were cross-validated above:
	// materialize the verified director-repo bytes into a fresh cache map
	// (never mutate the existing cache in place -- callers under the same
	// lock could be iterating it) and swap it in wholesale so subsequent
	// reads within the verify cache window are map lookups instead of
	// re-downloads.
	cachedTargetFiles := make(map[string][]byte, len(directorPathsDestinations))
	for targetPath, destination := range directorPathsDestinations {
		cachedTargetFiles[targetPath] = destination.(*bufferDestination).Bytes()
	}
	c.cachedDirectorTargets = directorTargets
	c.cachedTargetFiles = cachedTargetFiles
	return nil
}

func configMetasUpdateSummary(metas *pbgo.ConfigMetas) string {
	if metas == nil {
		return "no metas in update"
	}

	var b strings.Builder

	if len(metas.Roots) != 0 {
		b.WriteString("roots=")
		for i := 0; i < len(metas.Roots)-2; i++ {
			b.WriteString(fmt.Sprintf("%d,", metas.Roots[i].Version))
		}
		b.WriteString(fmt.Sprintf("%d ", metas.Roots[len(metas.Roots)-1].Version))
	}

	if metas.TopTargets != nil {
		b.WriteString(fmt.Sprintf("targets=%d ", metas.TopTargets.Version))
	}

	if metas.Snapshot != nil {
		b.WriteString(fmt.Sprintf("snapshot=%d ", metas.Snapshot.Version))
	}

	if metas.Timestamp != nil {
		b.WriteString(fmt.Sprintf("timestamp=%d", metas.Timestamp.Version))
	}

	return b.String()
}

// GetTransactionalStoreMetadata returns metadata that creates the underlying boltDB instance
func (c *Client) GetTransactionalStoreMetadata() (*Metadata, error) {
	return c.transactionalStore.getTSMetadata()
}
