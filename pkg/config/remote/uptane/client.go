// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package uptane

import (
	"bytes"
	"fmt"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/pkg/errors"
	"github.com/theupdateframework/go-tuf/client"
	"github.com/theupdateframework/go-tuf/data"
	"go.etcd.io/bbolt"
)

// State represents the state of an uptane client
type State struct {
	ConfigRootVersion     uint64
	ConfigSnapshotVersion uint64
	DirectorRootVersion   uint64
}

// Client is an uptane client
type Client struct {
	sync.Mutex

	orgIDTargetPrefix string

	configLocalStore  *localStoreConfig
	configRemoteStore *remoteStoreConfig
	configTUFClient   *client.Client

	directorLocalStore  *localStoreDirector
	directorRemoteStore *remoteStoreDirector
	directorTUFClient   *client.Client

	targetStore *targetStore
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
		orgIDTargetPrefix:   fmt.Sprintf("%d/", orgID),
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

// State returns the state of the uptane client
func (c *Client) State() (State, error) {
	c.Lock()
	defer c.Unlock()
	configRootVersion, err := c.configLocalStore.GetMetaVersion(metaRoot)
	if err != nil {
		return State{}, err
	}
	directorRootVersion, err := c.directorLocalStore.GetMetaVersion(metaRoot)
	if err != nil {
		return State{}, err
	}
	configSnapshotVersion, err := c.configLocalStore.GetMetaVersion(metaSnapshot)
	if err != nil {
		return State{}, err
	}
	return State{
		ConfigRootVersion:     configRootVersion,
		ConfigSnapshotVersion: configSnapshotVersion,
		DirectorRootVersion:   directorRootVersion,
	}, nil
}

// Targets returns the current targets of this uptane client
func (c *Client) Targets() (data.TargetFiles, error) {
	c.Lock()
	defer c.Unlock()
	return c.directorTUFClient.Targets()
}

// TargetsMeta returns the current raw targets.json meta of this uptane client
func (c *Client) TargetsMeta() ([]byte, error) {
	c.Lock()
	defer c.Unlock()
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

type bufferDestination struct {
	bytes.Buffer
}

func (b *bufferDestination) Delete() error {
	return nil
}

// TargetFile returns the content of a target if the repository is in a verified state
func (c *Client) TargetFile(path string) ([]byte, error) {
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

func (c *Client) updateRepos(response *pbgo.LatestConfigsResponse) error {
	err := c.targetStore.storeTargetFiles(response.TargetFiles)
	if err != nil {
		return err
	}
	c.directorRemoteStore.update(response)
	c.configRemoteStore.update(response)
	_, err = c.directorTUFClient.Update()
	if err != nil {
		return errors.Wrap(err, "could not update director repository")
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
	err := c.verifyOrgID()
	if err != nil {
		return err
	}
	return c.verifyUptane()
}

func (c *Client) verifyOrgID() error {
	directorTargets, err := c.directorTUFClient.Targets()
	if err != nil {
		return err
	}
	for targetPath := range directorTargets {
		if !strings.HasPrefix(targetPath, c.orgIDTargetPrefix) {
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
		for kind, directorHash := range targetMeta.Hashes {
			configHash, found := configTargetMeta.Hashes[kind]
			if !found {
				return fmt.Errorf("hash '%s' found in directory repository and not in config repository", directorHash)
			}
			if !bytes.Equal([]byte(directorHash), []byte(configHash)) {
				return fmt.Errorf("directory hash '%s' is not equal to config repository '%s'", string(directorHash), string(configHash))
			}
		}
	}
	return nil
}
