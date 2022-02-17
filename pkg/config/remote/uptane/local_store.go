// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package uptane

import (
	"encoding/json"
	fmt "fmt"

	"github.com/DataDog/datadog-agent/pkg/config/remote/meta"
	"go.etcd.io/bbolt"
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
// See https://pkg.go.dev/github.com/theupdateframework/go-tuf/client#LocalStore
type localStore struct {
	// metasBucket stores metadata saved by go-tuf
	metasBucket []byte
	// rootsBucket stores all the roots metadata ever saved by go-tuf
	// This is outside of the TUF specification but needed to update partial clients
	rootsBucket []byte
	db          *bbolt.DB
}

func newLocalStore(db *bbolt.DB, repository string, cacheKey string, initialRoots meta.EmbeddedRoots) (*localStore, error) {
	s := &localStore{
		db:          db,
		metasBucket: []byte(fmt.Sprintf("%s_%s_metas", cacheKey, repository)),
		rootsBucket: []byte(fmt.Sprintf("%s_%s_roots", cacheKey, repository)),
	}
	err := s.init(initialRoots)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *localStore) init(initialRoots meta.EmbeddedRoots) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(s.metasBucket)
		if err != nil {
			return fmt.Errorf("failed to create metas bucket: %v", err)
		}
		_, err = tx.CreateBucketIfNotExists(s.rootsBucket)
		if err != nil {
			return fmt.Errorf("failed to create roots bucket: %v", err)
		}
		for _, root := range initialRoots {
			err := s.writeRoot(tx, json.RawMessage(root))
			if err != nil {
				return fmt.Errorf("failed to set embedded root in roots bucket: %v", err)
			}
		}
		// This is the place where we pass embedded roots to go-tuf
		// Improvable if the API of go-tuf changes
		metasBucket := tx.Bucket(s.metasBucket)
		if metasBucket.Get([]byte(metaRoot)) == nil {
			err := metasBucket.Put([]byte(metaRoot), initialRoots.Last())
			if err != nil {
				return fmt.Errorf("failed to set embedded root in meta bucket: %v", err)
			}
		}
		return nil
	})
}

func (s *localStore) writeRoot(tx *bbolt.Tx, root json.RawMessage) error {
	version, err := metaVersion(root)
	if err != nil {
		return err
	}
	rootKey := []byte(fmt.Sprintf("%d.root.json", version))
	rootsBucket := tx.Bucket(s.rootsBucket)
	return rootsBucket.Put(rootKey, root)
}

// GetMeta implements go-tuf's LocalStore.GetTarget
// See https://pkg.go.dev/github.com/theupdateframework/go-tuf/client#LocalStore
func (s *localStore) GetMeta() (map[string]json.RawMessage, error) {
	meta := make(map[string]json.RawMessage)
	err := s.db.View(func(tx *bbolt.Tx) error {
		metaBucket := tx.Bucket(s.metasBucket)
		cursor := metaBucket.Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			tmp := make([]byte, len(v))
			copy(tmp, v)
			meta[string(k)] = json.RawMessage(tmp)
		}
		return nil
	})
	return meta, err
}

// DeleteMeta implements go-tuf's LocalStore.DeleteMeta
// See https://pkg.go.dev/github.com/theupdateframework/go-tuf/client#LocalStore
func (s *localStore) DeleteMeta(name string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		metaBucket := tx.Bucket(s.metasBucket)
		return metaBucket.Delete([]byte(name))
	})
}

// SetMeta implements go-tuf's LocalStore.SetMeta
// See https://pkg.go.dev/github.com/theupdateframework/go-tuf/client#LocalStore
func (s *localStore) SetMeta(name string, meta json.RawMessage) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		if name == metaRoot {
			err := s.writeRoot(tx, meta)
			if err != nil {
				return err
			}
		}
		metaBucket := tx.Bucket(s.metasBucket)
		return metaBucket.Put([]byte(name), meta)
	})
}

// GetRoot returns a version of the root metadata
func (s *localStore) GetRoot(version uint64) ([]byte, bool, error) {
	var root []byte
	err := s.db.View(func(tx *bbolt.Tx) error {
		rootsBucket := tx.Bucket(s.rootsBucket)
		r := rootsBucket.Get([]byte(fmt.Sprintf("%d.root.json", version)))
		root = append(root, r...)
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if len(root) == 0 {
		return nil, false, nil
	}
	return root, true, nil
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

// Close is a useless function required by go-tuf interface but unused in their code
func (s *localStore) Close() error {
	return nil
}

func newLocalStoreDirector(db *bbolt.DB, cacheKey string) (*localStore, error) {
	return newLocalStore(db, "director", cacheKey, meta.RootsDirector())
}

func newLocalStoreConfig(db *bbolt.DB, cacheKey string) (*localStore, error) {
	return newLocalStore(db, "config", cacheKey, meta.RootsConfig())
}
