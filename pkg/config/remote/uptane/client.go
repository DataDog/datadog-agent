// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package uptane contains the logic needed to perform the Uptane verification
// checks against stored TUF metadata and the associated config files.
package uptane

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/go-tuf/client"
	"github.com/DataDog/go-tuf/data"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"

	rdata "github.com/DataDog/datadog-agent/pkg/config/remote/data"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// Client is an uptane client
type Client struct {
	sync.Mutex

	site            string
	orgID           int64
	orgUUIDProvider OrgUUIDProvider

	configLocalStore   *localStore
	configRemoteStore  *remoteStoreConfig
	configTUFClient    *client.Client
	configRootOverride string

	directorLocalStore   *localStore
	directorRemoteStore  *remoteStoreDirector
	directorTUFClient    *client.Client
	directorRootOverride string

	targetStore *targetStore
	orgStore    *orgStore

	cachedVerify     bool
	cachedVerifyTime time.Time

	// TUF transaction tracker
	transactionalStore *transactionalStore
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

// NewClient creates a new uptane client
func NewClient(cacheDB *bbolt.DB, orgUUIDProvider OrgUUIDProvider, options ...ClientOption) (c *Client, err error) {
	transactionalStore := newTransactionalStore(cacheDB)
	targetStore := newTargetStore(transactionalStore)
	orgStore := newOrgStore(transactionalStore)

	c = &Client{
		configRemoteStore:   newRemoteStoreConfig(targetStore),
		directorRemoteStore: newRemoteStoreDirector(targetStore),
		targetStore:         targetStore,
		orgStore:            orgStore,
		transactionalStore:  transactionalStore,
		orgUUIDProvider:     orgUUIDProvider,
	}

	for _, o := range options {
		o(c)
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

// Update updates the uptane client and rollbacks in case of error
func (c *Client) Update(response *pbgo.LatestConfigsResponse) error {
	c.Lock()
	defer c.Unlock()
	c.cachedVerify = false

	// in case the commit is successful it is a no-op.
	// the defer is present to be sure a transaction is never left behind.
	defer c.transactionalStore.rollback()

	err := c.update(response)
	if err != nil {
		c.configRemoteStore = newRemoteStoreConfig(c.targetStore)
		c.directorRemoteStore = newRemoteStoreDirector(c.targetStore)
		c.configTUFClient = client.NewClient(c.configLocalStore, c.configRemoteStore)
		c.directorTUFClient = client.NewClient(c.directorLocalStore, c.directorRemoteStore)
		return err
	}
	return c.transactionalStore.commit()
}

// update updates the uptane client
func (c *Client) update(response *pbgo.LatestConfigsResponse) error {
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

// TargetsCustom returns the current targets custom of this uptane client
func (c *Client) TargetsCustom() ([]byte, error) {
	c.Lock()
	defer c.Unlock()
	return c.directorLocalStore.GetMetaCustom(metaTargets)
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

// TargetsMeta returns the current raw targets.json meta of this uptane client
func (c *Client) TargetsMeta() ([]byte, error) {
	c.Lock()
	defer c.Unlock()
	err := c.verify()
	if err != nil {
		return nil, err
	}
	metas, err := c.directorLocalStore.GetMeta()
	if err != nil {
		return nil, err
	}
	targets, found := metas[metaTargets]
	if !found {
		return nil, fmt.Errorf("empty targets meta in director local store")
	}
	return targets, nil
}

func (c *Client) updateRepos(response *pbgo.LatestConfigsResponse) error {
	err := c.targetStore.storeTargetFiles(response.TargetFiles)
	if err != nil {
		return err
	}
	c.directorRemoteStore.update(response)
	c.configRemoteStore.update(response)
	_, err = c.directorTUFClient.Update()
	if err != nil {
		return errors.Wrap(err, "failed updating director repository")
	}
	_, err = c.configTUFClient.Update()
	if err != nil {
		e := fmt.Sprintf("could not update config repository [%s]", configMetasUpdateSummary(response.ConfigMetas))
		return errors.Wrap(err, e)
	}
	return nil
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
			return fmt.Errorf("director target '%s' does not have the correct orgID", targetPath)
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
		return nil
	}

	targetPathsDestinations := make(map[string]client.Destination)
	targetPaths := make([]string, 0, len(directorTargets))
	for targetPath := range directorTargets {
		targetPaths = append(targetPaths, targetPath)
		targetPathsDestinations[targetPath] = &bufferDestination{}
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
	err = c.configTUFClient.DownloadBatch(targetPathsDestinations)
	if err != nil {
		return err
	}
	err = c.directorTUFClient.DownloadBatch(targetPathsDestinations)
	if err != nil {
		return err
	}
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
