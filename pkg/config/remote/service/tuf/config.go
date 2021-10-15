// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tuf

import (
	"bytes"
	"io"
	"io/ioutil"

	"github.com/theupdateframework/go-tuf/client"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote/service/meta"
	"github.com/DataDog/datadog-agent/pkg/config/remote/store"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
)

type configLocalStore struct {
	*localBoltStore
}

func newConfigLocalStore(store *store.Store) *configLocalStore {
	return &configLocalStore{
		localBoltStore: &localBoltStore{
			name:  "config",
			store: store,
		},
	}
}

func getConfigRoot() []byte {
	if configRoot := config.Datadog.GetString("remote_configuration.config_root"); configRoot != "" {
		return []byte(configRoot)
	}
	return meta.RootConfig
}

type configRemoteStore struct {
	configMetas pbgo.ConfigMetas
	targetFiles []*pbgo.File
}

// GetMeta downloads the given metadata from remote storage.
//
// `name` is the filename of the metadata (e.g. "root.json")
//
// `err` is ErrNotFound if the given file does not exist.
//
// `size` is the size of the stream, -1 indicating an unknown length.
func (s *configRemoteStore) GetMeta(name string) (stream io.ReadCloser, size int64, _ error) {
	var content []byte

	version, role, err := parseMeta(name)
	if err != nil {
		return nil, 0, err
	}

	if role == "root" && version < 1 {
		content = getConfigRoot()
	} else {
		switch {
		case role == "root":
			if version > len(s.configMetas.Roots) || s.configMetas.Roots[version-1].Version != uint64(version) {
				return nil, 0, client.ErrNotFound{File: name}
			}
			content = s.configMetas.Roots[version-1].Raw
		case role == "snapshot":
			if s.configMetas.Snapshot.Version != uint64(version) {
				return nil, 0, client.ErrNotFound{File: name}
			}
			content = s.configMetas.Snapshot.Raw
		case role == "timestamp":
			content = s.configMetas.Timestamp.Raw
		case role == "targets":
			if s.configMetas.TopTargets.Version != uint64(version) {
				return nil, 0, client.ErrNotFound{File: name}
			}
			content = s.configMetas.TopTargets.Raw
		default:
			found := false
			for _, delegatedTarget := range s.configMetas.DelegatedTargets {
				if role == delegatedTarget.Role && delegatedTarget.Version == uint64(version) {
					found = true
					content = delegatedTarget.Raw
					break
				}
			}
			if !found {
				return nil, 0, client.ErrNotFound{File: name}
			}
		}
	}

	return ioutil.NopCloser(bytes.NewReader(content)), int64(len(content)), nil
}

// GetTarget downloads the given target file from remote storage.
//
// `path` is the path of the file relative to the root of the remote
//        targets directory (e.g. "/path/to/file.txt").
//
// `err` is ErrNotFound if the given file does not exist.
//
// `size` is the size of the stream, -1 indicating an unknown length.
func (s *configRemoteStore) GetTarget(path string) (stream io.ReadCloser, size int64, err error) {
	for _, file := range s.targetFiles {
		if path == file.Path {
			return ioutil.NopCloser(bytes.NewReader(file.Raw)), int64(len(file.Raw)), nil
		}
	}
	return nil, 0, client.ErrNotFound{File: path}
}

// ConfigClient is a TUF client that performs full verification
// against a config repository. Remote store is bound to the API response
// while local store is bound to both go-bindata bundles and local
// boltdb database
type ConfigClient struct {
	*client.Client
	local  *configLocalStore
	remote *configRemoteStore
}

func (c *ConfigClient) updateRemote(response *pbgo.LatestConfigsResponse) {
	if response.ConfigMetas != nil {
		if response.ConfigMetas.Timestamp != nil {
			c.remote.configMetas.Timestamp = response.ConfigMetas.Timestamp
		}
		if response.ConfigMetas.Snapshot != nil {
			c.remote.configMetas.Snapshot = response.ConfigMetas.Snapshot
		}
		if len(response.ConfigMetas.Roots) != 0 {
			c.remote.configMetas.Roots = append(c.remote.configMetas.Roots, response.ConfigMetas.Roots...)
		}
		if response.ConfigMetas.TopTargets != nil {
			c.remote.configMetas.TopTargets = response.ConfigMetas.TopTargets
		}
		if response.ConfigMetas.DelegatedTargets != nil {
			c.remote.configMetas.DelegatedTargets = response.ConfigMetas.DelegatedTargets
		}
	}
	c.remote.targetFiles = response.TargetFiles
}

// Update the remote with data from the response and verify the TUF metadata
func (c *ConfigClient) Update(response *pbgo.LatestConfigsResponse) error {
	c.updateRemote(response)

	_, err := c.Client.Update()
	return err
}

// NewConfigClient returns a new TUF client for config repository
func NewConfigClient(store *store.Store) *ConfigClient {
	local := newConfigLocalStore(store)
	remote := &configRemoteStore{}
	return &ConfigClient{
		Client: client.NewClient(local, remote),
		local:  local,
		remote: remote,
	}
}
