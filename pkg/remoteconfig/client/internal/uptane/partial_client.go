// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package uptane

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"

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

// PartialClient is a partial uptane client
type PartialClient struct {
	rootClient       *client.Client
	rootVersion      int64
	rootsLocalStore  client.LocalStore
	rootsRemoteStore *partialClientRemoteStore
}

// NewPartialClient creates a new partial uptane client
func NewPartialClient(embededRoot []byte) *PartialClient {
	embededRootVersion, err := metaVersion(json.RawMessage(embededRoot))
	if err != nil {
		panic(err)
	}
	localStore := client.MemoryLocalStore()
	err = localStore.SetMeta("root.json", json.RawMessage(embededRoot))
	if err != nil {
		panic(err) // the memory store can not error
	}
	remoteStore := &partialClientRemoteStore{}
	c := &PartialClient{
		rootClient:       client.NewClient(localStore, remoteStore),
		rootsLocalStore:  localStore,
		rootsRemoteStore: remoteStore,
		rootVersion:      embededRootVersion,
	}
	return c
}

func (c *PartialClient) getRoot() (*data.Root, error) {
	metas, err := c.rootsLocalStore.GetMeta()
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

// RootVersion returns the state of the partial client
func (c *PartialClient) RootVersion() int64 {
	return c.rootVersion
}

// UpdateRoots updates the partial client roots
func (c *PartialClient) UpdateRoots(roots [][]byte) error {
	if len(roots) == 0 {
		return nil
	}
	c.rootsRemoteStore.roots = roots
	err := c.rootClient.UpdateRoots()
	if err != nil {
		return err
	}
	return c.updateRootVersion()
}

// PartialClientTargets is a partial client targets
type PartialClientTargets struct {
	version     int64
	metas       data.TargetFiles
	targetFiles map[string][]byte
}

// Targets returns the current targets of this uptane partial client
func (t *PartialClientTargets) Targets() data.TargetFiles {
	return t.metas
}

// TargetFile returns the content of a target
func (t *PartialClientTargets) TargetFile(path string) ([]byte, bool) {
	file, found := t.targetFiles[path]
	return file, found
}

func mergeTargetFiles(old map[string][]byte, new map[string][]byte) map[string][]byte {
	newTargetFiles := make(map[string][]byte)
	for path, target := range old {
		newTargetFiles[path] = target
	}
	for path, target := range new {
		newTargetFiles[path] = target
	}
	return newTargetFiles
}

func purgeTargetFiles(tufTargetFiles data.TargetFiles, targetFiles map[string][]byte) {
	for path := range targetFiles {
		if _, found := tufTargetFiles[path]; !found {
			delete(targetFiles, path)
		}
	}
}

// UpdateTargets updates the partial client
func (c *PartialClient) UpdateTargets(previousTargets *PartialClientTargets, rawTargets []byte, targetFiles map[string][]byte) (*PartialClientTargets, error) {
	if len(rawTargets) == 0 {
		return previousTargets, nil
	}
	targets, err := c.validateTargets(rawTargets)
	if err != nil {
		return nil, err
	}
	mergedTargetFiles := mergeTargetFiles(previousTargets.targetFiles, targetFiles)
	for path, targetMeta := range targets.Targets {
		targetFile, found := mergedTargetFiles[path]
		if !found {
			continue
		}
		err := validateTargetFile(targetMeta, targetFile)
		if err != nil {
			return nil, err
		}
	}
	purgeTargetFiles(targets.Targets, mergedTargetFiles)
	return &PartialClientTargets{
		version:     int64(targets.Version),
		metas:       targets.Targets,
		targetFiles: mergedTargetFiles,
	}, nil
}

func (c *PartialClient) updateRootVersion() error {
	meta, err := c.rootsLocalStore.GetMeta()
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

func (c *PartialClient) validateTargets(rawTargets []byte) (*data.Targets, error) {
	root, err := c.getRoot()
	if err != nil {
		return nil, err
	}
	db := verify.NewDB()
	for _, key := range root.Keys {
		for _, id := range key.IDs() {
			if err := db.AddKey(id, key); err != nil {
				return nil, err
			}
		}
	}
	targetsRole, hasRoleTargets := root.Roles["targets"]
	if !hasRoleTargets {
		return nil, fmt.Errorf("root is missing a targets role")
	}
	role := &data.Role{Threshold: targetsRole.Threshold, KeyIDs: targetsRole.KeyIDs}
	if err := db.AddRole("targets", role); err != nil {
		return nil, fmt.Errorf("could not add targets role to db: %v", err)
	}
	var targets data.Targets
	err = db.Unmarshal(rawTargets, &targets, "targets", 0)
	if err != nil {
		return nil, err
	}
	return &targets, nil
}

func validateTargetFile(targetMeta data.TargetFileMeta, targetFile []byte) error {
	if len(targetMeta.HashAlgorithms()) == 0 {
		return fmt.Errorf("target file has no hash")
	}
	generatedMeta, err := util.GenerateFileMeta(bytes.NewBuffer(targetFile), targetMeta.HashAlgorithms()...)
	if err != nil {
		return err
	}
	err = util.FileMetaEqual(targetMeta.FileMeta, generatedMeta)
	if err != nil {
		return err
	}
	return nil
}
