// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config/remote/meta"
	"github.com/theupdateframework/go-tuf/client"
	"github.com/theupdateframework/go-tuf/data"
	"github.com/theupdateframework/go-tuf/util"
	"github.com/theupdateframework/go-tuf/verify"
)

type partialClientRemoteStore struct {
	roots [][]byte
}

func (s *partialClientRemoteStore) GetMeta(name string) (stream io.ReadCloser, size int64, err error) {
	metaPath, err := parseMetaPath(name)
	if err != nil {
		return nil, 0, err
	}
	if metaPath.role != roleRoot || !metaPath.versionSet {
		return nil, 0, client.ErrNotFound{File: name}
	}
	for _, root := range s.roots {
		version, err := metaVersion(root)
		if err != nil {
			return nil, 0, err
		}
		if version == metaPath.version {
			return ioutil.NopCloser(bytes.NewReader(root)), int64(len(root)), nil
		}
	}
	return nil, 0, client.ErrNotFound{File: name}
}

func (s *partialClientRemoteStore) GetTarget(path string) (stream io.ReadCloser, size int64, err error) {
	return nil, 0, client.ErrNotFound{File: path}
}

// PartialState represents the state of a partial uptane client
type PartialState struct {
	RootVersion    uint64
	TargetsVersion uint64
}

// PartialClient is a partial uptane client
type PartialClient struct {
	sync.Mutex

	rootClient  *client.Client
	localStore  client.LocalStore
	remoteStore *partialClientRemoteStore

	valid bool

	rootVersion    uint64
	targetsVersion uint64
	targetMetas    data.TargetFiles
	targetFiles    map[string][]byte
}

// NewPartialClient creates a new partial uptane client
func NewPartialClient() (*PartialClient, error) {
	localStore := client.MemoryLocalStore()
	err := localStore.SetMeta("root.json", json.RawMessage(meta.RootsDirector().Last()))
	if err != nil {
		return nil, err
	}
	remoteStore := &partialClientRemoteStore{}
	c := &PartialClient{
		rootClient:  client.NewClient(localStore, remoteStore),
		localStore:  localStore,
		remoteStore: remoteStore,
		rootVersion: meta.RootsDirector().LastVersion(),
	}
	return c, nil
}

func (c *PartialClient) getRoot() (*data.Root, error) {
	metas, err := c.localStore.GetMeta()
	if err != nil {
		return nil, err
	}
	rawRoot := metas["root.json"]
	var signedRoot data.Signed
	err = json.Unmarshal(rawRoot, &signedRoot)
	if err != nil {
		return nil, err
	}
	var root data.Root
	err = json.Unmarshal(signedRoot.Signed, &root)
	if err != nil {
		return nil, err
	}
	return &root, nil
}

func (c *PartialClient) validateAndUpdateTargets(rawTargets []byte) error {
	if len(rawTargets) == 0 {
		return nil
	}
	root, err := c.getRoot()
	if err != nil {
		return err
	}
	db := verify.NewDB()
	for _, key := range root.Keys {
		for _, id := range key.IDs() {
			if err := db.AddKey(id, key); err != nil {
				return err
			}
		}
	}
	targetsRole, hasRoleTargets := root.Roles["targets"]
	if !hasRoleTargets {
		return fmt.Errorf("root is missing a targets role")
	}
	role := &data.Role{Threshold: targetsRole.Threshold, KeyIDs: targetsRole.KeyIDs}
	if err := db.AddRole("targets", role); err != nil {
		return fmt.Errorf("could not add targets role to db: %v", err)
	}
	var targets data.Targets
	err = db.Unmarshal(rawTargets, &targets, "targets", 0)
	if err != nil {
		return err
	}
	c.targetMetas = targets.Targets
	c.targetsVersion = uint64(targets.Version)
	return nil
}

// State returns the state of the partial client
func (c *PartialClient) State() PartialState {
	c.Lock()
	defer c.Unlock()
	return PartialState{
		RootVersion:    c.rootVersion,
		TargetsVersion: c.targetsVersion,
	}
}

// Update updates the partial client
func (c *PartialClient) Update(update Update) error {
	c.valid = false
	c.remoteStore.roots = update.Roots
	err := c.rootClient.UpdateRoots()
	if err != nil {
		return err
	}
	err = c.updateRootVersion()
	if err != nil {
		return err
	}
	if len(update.Targets) == 0 {
		c.valid = true
		return nil
	}
	err = c.validateAndUpdateTargets(update.Targets)
	if err != nil {
		return err
	}
	for _, target := range update.TargetFiles {
		c.targetFiles[target.Path] = target.Raw
		_, err := c.targetFile(target.Path)
		if err != nil {
			return err
		}
	}
	for filePath := range c.targetFiles {
		_, exist := c.targetMetas[filePath]
		if !exist {
			delete(c.targetFiles, filePath)
		}
	}
	c.valid = true
	return nil
}

func (c *PartialClient) updateRootVersion() error {
	meta, err := c.localStore.GetMeta()
	if err != nil {
		return err
	}
	rootMeta, rootFound := meta["root.json"]
	if !rootFound {
		return fmt.Errorf("could not find root.json in the local store")
	}
	version, err := metaVersion(rootMeta)
	if err != nil {
		return err
	}
	c.rootVersion = version
	return nil
}

// Targets returns the current targets of this uptane partial client
func (c *PartialClient) Targets() (data.TargetFiles, error) {
	if !c.valid {
		return nil, fmt.Errorf("partial client local repository is not in a valid state")
	}
	return c.targetMetas, nil
}

// TargetFile returns the content of a target
func (c *PartialClient) TargetFile(path string) ([]byte, error) {
	if !c.valid {
		return nil, fmt.Errorf("partial client local repository is not in a valid state")
	}
	return c.targetFile(path)
}

func (c *PartialClient) targetFile(path string) ([]byte, error) {
	targetFile, found := c.targetFiles[path]
	if !found {
		return nil, fmt.Errorf("target file %s not found", path)
	}
	targetMeta, hasMeta := c.targetMetas[path]
	if !hasMeta {
		return nil, fmt.Errorf("target file meta %s not found", path)
	}
	if len(targetMeta.HashAlgorithms()) == 0 {
		return nil, fmt.Errorf("target file %s has no hash", path)
	}
	generatedMeta, err := util.GenerateFileMeta(bytes.NewBuffer(targetFile), targetMeta.HashAlgorithms()...)
	if err != nil {
		return nil, err
	}
	err = util.FileMetaEqual(targetMeta.FileMeta, generatedMeta)
	if err != nil {
		return nil, err
	}
	return targetFile, nil
}
