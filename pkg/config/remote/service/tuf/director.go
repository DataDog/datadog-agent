// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tuf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/theupdateframework/go-tuf/client"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote/service/meta"
	"github.com/DataDog/datadog-agent/pkg/config/remote/store"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
)

type directorLocalStore struct {
	*localBoltStore
}

func newDirectorLocalStore(store *store.Store) *directorLocalStore {
	return &directorLocalStore{
		localBoltStore: &localBoltStore{
			name:  "director",
			store: store,
		},
	}
}

func getDirectorRoot() []byte {
	if directorRoot := config.Datadog.GetString("remote_configuration.director_root"); directorRoot != "" {
		return []byte(directorRoot)
	}
	return meta.RootDirector
}

// directorRemoteStore implements the go-tuf remote store interface
// by request data from the response sent by the backend
type directorRemoteStore struct {
	directorMetas pbgo.DirectorMetas
	targetFiles   []*pbgo.File
}

func parseMeta(name string) (version int, role string, _ error) {
	splits := strings.SplitN(name, ".", 3)
	if len(splits) == 3 {
		parsedVersion, err := strconv.Atoi(splits[0])
		if err != nil {
			return 0, "", fmt.Errorf("invalid version of metadata '%s': %w", name, err)
		}
		version, role = parsedVersion, splits[1]
	} else if len(splits) == 2 {
		role = splits[0]
	} else {
		return 0, "", fmt.Errorf("invalid metadata name '%s'", name)
	}

	if role == "" {
		return 0, "", fmt.Errorf("invalid metadata name '%s'", name)
	}

	return version, role, nil
}

// GetMeta downloads the given metadata from remote storage.
//
// `name` is the filename of the metadata. It can be either
//        the name of a role ending with .json such as "root.json" when asking for
//        the current version, or <VERSION>.<ROLE>.json to request a
//        particular version 3.root.json
//
// `err` is ErrNotFound if the given file does not exist.
//
// `size` is the size of the stream, -1 indicating an unknown length.
func (s *directorRemoteStore) GetMeta(name string) (stream io.ReadCloser, size int64, err error) {
	var content []byte

	version, role, err := parseMeta(name)
	if err != nil {
		return nil, 0, err
	}

	if role == "root" && version < 1 {
		content = getDirectorRoot()
	} else {
		switch {
		case role == "root":
			if version > len(s.directorMetas.Roots) || s.directorMetas.Roots[version-1].Version != uint64(version) {
				return nil, 0, client.ErrNotFound{File: name}
			}
			// TODO(sbaubeau): should we look for the version in the root array ?
			content = s.directorMetas.Roots[version-1].Raw
		case role == "snapshot":
			if s.directorMetas.Snapshot == nil || s.directorMetas.Snapshot.Version != uint64(version) {
				return nil, 0, client.ErrNotFound{File: name}
			}
			content = s.directorMetas.Snapshot.Raw
		case role == "timestamp":
			if s.directorMetas.Timestamp == nil {
				return nil, 0, client.ErrNotFound{File: name}
			}
			content = s.directorMetas.Timestamp.Raw
		case role == "targets":
			if s.directorMetas.Targets == nil || s.directorMetas.Targets.Version != uint64(version) {
				return nil, 0, client.ErrNotFound{File: name}
			}
			content = s.directorMetas.Targets.Raw
		default:
			return nil, 0, client.ErrNotFound{File: name}
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
func (s *directorRemoteStore) GetTarget(path string) (stream io.ReadCloser, size int64, err error) {
	for _, file := range s.targetFiles {
		if path == file.Path {
			return ioutil.NopCloser(bytes.NewReader(file.Raw)), int64(len(file.Raw)), nil
		}
	}
	return nil, 0, client.ErrNotFound{File: path}
}

// DirectorClient is a TUF client that performs full verification
// against a director repository. Remote store is bound to the API response
// while local store is bound to both go-bindata bundles and local
// boltdb database
type DirectorClient struct {
	*client.Client
	local  client.LocalStore
	remote *directorRemoteStore
}

func (c *DirectorClient) updateRemote(response *pbgo.LatestConfigsResponse) {
	if response != nil {
		c.remote.targetFiles = response.TargetFiles

		if response.DirectorMetas != nil {
			if response.DirectorMetas.Timestamp != nil {
				c.remote.directorMetas.Timestamp = response.DirectorMetas.Timestamp
			}
			if response.DirectorMetas.Snapshot != nil {
				c.remote.directorMetas.Snapshot = response.DirectorMetas.Snapshot
			}
			if len(response.DirectorMetas.Roots) != 0 {
				c.remote.directorMetas.Roots = append(c.remote.directorMetas.Roots, response.DirectorMetas.Roots...)
			}
			if response.DirectorMetas.Targets != nil {
				c.remote.directorMetas.Targets = response.DirectorMetas.Targets
			}
		}
	}
}

// Update the remote with data from the response and verifiers the TUF metadata
func (c *DirectorClient) Update(response *pbgo.LatestConfigsResponse) error {
	c.updateRemote(response)

	_, err := c.Client.Update()
	return err
}

// NewDirectorClient returns a new TUF client for director repository
func NewDirectorClient(store *store.Store) *DirectorClient {
	local := newDirectorLocalStore(store)
	remote := &directorRemoteStore{}
	return &DirectorClient{
		Client: client.NewClient(local, remote),
		local:  local,
		remote: remote,
	}
}

type nullDestination struct {
}

func (d *nullDestination) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (d *nullDestination) Delete() error {
	return nil
}

// DirectorPartialClient is a TUF client that performs partial verification
// against a director repository as described in the update specification.
// Remote store is bound to the API response while local store is bound
// to both go-bindata bundles and local boltdb database
type DirectorPartialClient struct {
	local  client.LocalStore
	remote *directorRemoteStore
}

// TrimHash trim the file hash from the name
// (eg. changing my-product/1234.my.target into my-product/my.target)
func TrimHash(path string) string {
	basename := filepath.Base(path)
	split := strings.SplitN(basename, ".", 2)
	if len(split) > 1 {
		basename = split[1]
	}
	return filepath.Join(filepath.Dir(path), basename)
}

// Verify partially the configuration
func (c *DirectorPartialClient) Verify(response *pbgo.ConfigResponse) error {
	// We create a new client for every verification because the go-tuf
	// targets are cached and only updated with an Update that we don't do
	// for a partial verification
	client := client.NewClient(c.local, c.remote)

	if response != nil {
		c.remote.targetFiles = response.TargetFiles
		c.remote.directorMetas = pbgo.DirectorMetas{
			Roots:   response.DirectoryRoots,
			Targets: response.DirectoryTargets,
		}
	}

	if meta, err := c.local.GetMeta(); err == nil {
		if _, found := meta["root.json"]; !found {
			root := getDirectorRoot()
			if err := c.local.SetMeta("root.json", json.RawMessage(root)); err != nil {
				return err
			}
		}
	}

	if err := c.local.SetMeta("targets.json", response.DirectoryTargets.Raw); err != nil {
		return err
	}

	for _, targetFile := range response.TargetFiles {
		path := TrimHash(targetFile.Path)

		// TODO(sbaubeau): terrible hack to do update root rotation.
		// You could either call updateRoots in Download, expose the UpdateRoots() method
		// or
		_, _ = client.Update()
		if err := client.Download(path, &nullDestination{}); err != nil {
			return err
		}
	}

	return nil
}

// NewDirectorPartialClient returns a new TUF client that performs partial validation
func NewDirectorPartialClient() *DirectorPartialClient {
	local := client.MemoryLocalStore()
	remote := &directorRemoteStore{}
	return &DirectorPartialClient{
		local:  local,
		remote: remote,
	}
}
