// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package uptane

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	rdata "github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/pkg/errors"
	"github.com/theupdateframework/go-tuf/client"
	"github.com/theupdateframework/go-tuf/data"
	"go.etcd.io/bbolt"
)

// Client is an uptane client
type Client struct {
	sync.Mutex

	orgID int64

	configLocalStore  *localStore
	configRemoteStore *remoteStoreConfig
	configTUFClient   *client.Client

	directorLocalStore  *localStore
	directorRemoteStore *remoteStoreDirector
	directorTUFClient   *client.Client

	targetStore *targetStore

	cachedVerify     bool
	cachedVerifyTime time.Time
}

// NewClient creates a new uptane client
func NewClient(cacheDB *bbolt.DB, cacheKey string, orgID int64) (*Client, error) {
	localStoreConfig, err := newLocalStoreConfig(cacheDB, cacheKey)
	if err != nil {
		return nil, err
	}
	localStoreDirector, err := newLocalStoreDirector(cacheDB, cacheKey)
	if err != nil {
		return nil, err
	}
	targetStore, err := newTargetStore(cacheDB, cacheKey)
	if err != nil {
		return nil, err
	}
	c := &Client{
		orgID:               orgID,
		configLocalStore:    localStoreConfig,
		configRemoteStore:   newRemoteStoreConfig(targetStore),
		directorLocalStore:  localStoreDirector,
		directorRemoteStore: newRemoteStoreDirector(targetStore),
		targetStore:         targetStore,
	}
	c.configTUFClient = client.NewClient(c.configLocalStore, c.configRemoteStore)
	c.directorTUFClient = client.NewClient(c.directorLocalStore, c.directorRemoteStore)
	return c, nil
}

// Update updates the uptane client
func (c *Client) Update(response *pbgo.LatestConfigsResponse) error {
	c.Lock()
	defer c.Unlock()
	c.cachedVerify = false
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
	err = c.configTUFClient.Download(path, buffer)
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
		return errors.Wrap(err, "could not update config repository")
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
	err := c.verifyOrgID()
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

func (c *Client) verifyOrgID() error {
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
	for targetPath, targetMeta := range directorTargets {
		configTargetMeta, err := c.configTUFClient.Target(targetPath)
		if err != nil {
			return fmt.Errorf("failed to find target '%s' in config repository", targetPath)
		}
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
		// Check that the file is valid in the context of the TUF repository (path in targets, hash matching)
		err = c.configTUFClient.Download(targetPath, &bufferDestination{})
		if err != nil {
			return err
		}
		err = c.directorTUFClient.Download(targetPath, &bufferDestination{})
		if err != nil {
			return err
		}
	}
	return nil
}
