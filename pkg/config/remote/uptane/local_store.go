// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package uptane

import (
	"encoding/json"
	fmt "fmt"

	"github.com/DataDog/datadog-agent/pkg/config/remote/meta"
)

var (
	metaRoot     = "root.json"
	metaTargets  = "targets.json"
	metaSnapshot = "snapshot.json"
)

// localStore implements go-tuf's LocalStore
// Its goal is to persist TUF metadata. This implementation of the local store
// also saves every root ever validated by go-tuf. This is needed to update the roots
// of tracers and other partial clients.
// See https://pkg.go.dev/github.com/DataDog/go-tuf/client#LocalStore
type localStore struct {
	// metasBucket stores metadata saved by go-tuf
	metasBucket string
	// rootsBucket stores all the roots metadata ever saved by go-tuf
	// This is outside of the TUF specification but needed to update partial clients
	rootsBucket string

	store *transactionalStore
}

func newLocalStore(db *transactionalStore, repository string, initialRoots meta.EmbeddedRoots) (*localStore, error) {
	s := &localStore{
		store:       db,
		metasBucket: fmt.Sprintf("%s_metas", repository),
		rootsBucket: fmt.Sprintf("%s_roots", repository),
	}
	err := s.init(initialRoots)
	return s, err
}

func (s *localStore) init(initialRoots meta.EmbeddedRoots) error {
	err := s.store.update(func(tx *transaction) error {
		for _, root := range initialRoots {
			err := s.writeRoot(tx, json.RawMessage(root))
			if err != nil {
				return fmt.Errorf("failed to set embedded root in roots bucket: %v", err)
			}
		}

		data, err := tx.get(s.metasBucket, metaRoot)
		if err != nil {
			return err
		}
		if data == nil {
			tx.put(s.metasBucket, metaRoot, initialRoots.Last())
		}
		return nil
	})
	if err != nil {
		s.store.rollback()
		return err
	}
	return s.store.commit()
}

func (s *localStore) writeRoot(tx *transaction, root json.RawMessage) error {
	version, err := metaVersion(root)
	if err != nil {
		return err
	}
	rootKey := fmt.Sprintf("%d.root.json", version)
	tx.put(s.rootsBucket, rootKey, root)
	return nil
}

// GetMeta implements go-tuf's LocalStore.GetTarget
// See https://pkg.go.dev/github.com/DataDog/go-tuf/client#LocalStore
func (s *localStore) GetMeta() (map[string]json.RawMessage, error) {
	meta := make(map[string]json.RawMessage)
	err := s.store.view(func(tx *transaction) error {
		allFiles, err := tx.getAll(s.metasBucket)
		if err != nil {
			return err
		}
		for _, blob := range allFiles {
			meta[blob.path] = json.RawMessage(blob.data)
		}
		return nil
	})
	return meta, err
}

// DeleteMeta implements go-tuf's LocalStore.DeleteMeta
// See https://pkg.go.dev/github.com/DataDog/go-tuf/client#LocalStore
func (s *localStore) DeleteMeta(name string) error {
	return s.store.update(func(tx *transaction) error {
		tx.delete(s.metasBucket, name)
		return nil
	})
}

// SetMeta implements go-tuf's LocalStore.SetMeta
// See https://pkg.go.dev/github.com/DataDog/go-tuf/client#LocalStore
func (s *localStore) SetMeta(name string, meta json.RawMessage) error {
	return s.store.update(func(tx *transaction) error {
		if name == metaRoot {
			err := s.writeRoot(tx, meta)
			if err != nil {
				return err
			}
		}
		tx.put(s.metasBucket, name, meta)
		return nil
	})
}

// GetRoot returns a version of the root metadata
func (s *localStore) GetRoot(version uint64) ([]byte, bool, error) {
	var root []byte
	err := s.store.view(func(tx *transaction) error {
		r, err := tx.get(s.rootsBucket, fmt.Sprintf("%d.root.json", version))
		if err != nil {
			return err
		}
		root = r
		return nil
	})
	return root, len(root) != 0, err
}

// GetMetaVersion returns the latest version of a particular meta
func (s *localStore) GetMetaVersion(metaName string) (uint64, error) {
	metas, err := s.GetMeta()
	if err != nil {
		return 0, err
	}
	meta, found := metas[metaName]
	if !found {
		return 0, nil
	}
	metaVersion, err := metaVersion(meta)
	if err != nil {
		return 0, err
	}
	return metaVersion, nil
}

// GetMetaCustom returns the custom of a particular meta
func (s *localStore) GetMetaCustom(metaName string) ([]byte, error) {
	metas, err := s.GetMeta()
	if err != nil {
		return nil, err
	}
	meta, found := metas[metaName]
	if !found {
		return nil, nil
	}
	return metaCustom(meta)
}

// Close commits all pending data to the stored database
func (s *localStore) Close() error {
	return s.Flush()
}

// Flush flushes all data to disk
func (s *localStore) Flush() error {
	return s.store.commit()
}

func newLocalStoreDirector(db *transactionalStore, site string, directorRootOverride string) (*localStore, error) {
	return newLocalStore(db, "director", meta.RootsDirector(site, directorRootOverride))
}

func newLocalStoreConfig(db *transactionalStore, site string, configRootOverride string) (*localStore, error) {
	return newLocalStore(db, "config", meta.RootsConfig(site, configRootOverride))
}
