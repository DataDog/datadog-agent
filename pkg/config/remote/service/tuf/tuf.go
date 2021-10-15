// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tuf

import (
	"encoding/json"
	"errors"
	"fmt"

	"go.etcd.io/bbolt"

	"github.com/DataDog/datadog-agent/pkg/config/remote/store"
)

type localBoltStore struct {
	name  string
	store *store.Store
}

// GetMeta returns top-level metadata from local storage. The keys are
// in the form `ROLE.json`, with ROLE being a valid top-level role.
func (s *localBoltStore) GetMeta() (map[string]json.RawMessage, error) {
	meta, err := s.store.GetMeta(s.name)
	if err != nil {
		if !errors.Is(err, bbolt.ErrBucketNotFound) {
			return nil, err
		}
		meta = make(map[string]json.RawMessage)
	}

	if _, found := meta["root.json"]; !found {
		var rootMetadata []byte
		var err error
		switch s.name {
		case "director":
			rootMetadata = getDirectorRoot()
		case "config":
			rootMetadata = getConfigRoot()
		default:
			return nil, fmt.Errorf("unexpected root name")
		}
		if err != nil {
			return nil, err
		}
		meta["root.json"] = rootMetadata
	}

	return meta, nil
}

// SetMeta persists the given top-level metadata in local storage, the
// name taking the same format as the keys returned by GetMeta.
func (s *localBoltStore) SetMeta(name string, meta json.RawMessage) error {
	return s.store.SetMeta(s.name, name, meta)
}

// DeleteMeta
func (s *localBoltStore) DeleteMeta(name string) error {
	return s.store.DeleteMeta(s.name, name)
}
