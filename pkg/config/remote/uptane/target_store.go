// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package uptane

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"go.etcd.io/bbolt"
)

// targetStore persists all the target files present in the current director targets.json
type targetStore struct {
	db           *bbolt.DB
	targetBucket []byte
}

func newTargetStore(db *bbolt.DB, cacheKey string) (*targetStore, error) {
	s := &targetStore{
		db:           db,
		targetBucket: []byte(fmt.Sprintf("%s_targets", cacheKey)),
	}
	err := s.init()
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *targetStore) init() error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(s.targetBucket)
		if err != nil {
			return fmt.Errorf("failed to create targets bucket: %v", err)
		}
		return nil
	})
}

func (s *targetStore) storeTargetFiles(targetFiles []*pbgo.File) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		targetBucket := tx.Bucket(s.targetBucket)
		for _, target := range targetFiles {
			err := targetBucket.Put([]byte(trimHashTargetPath(target.Path)), target.Raw)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *targetStore) getTargetFile(path string) ([]byte, bool, error) {
	var target []byte
	err := s.db.View(func(tx *bbolt.Tx) error {
		targetBucket := tx.Bucket(s.targetBucket)
		t := targetBucket.Get([]byte(trimHashTargetPath(path)))
		target = append(target, t...)
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if len(target) == 0 {
		return nil, false, nil
	}
	return target, true, nil
}

func (s *targetStore) pruneTargetFiles(keptPaths []string) error {
	kept := make(map[string]struct{})
	for _, k := range keptPaths {
		kept[k] = struct{}{}
	}
	return s.db.Update(func(tx *bbolt.Tx) error {
		targetBucket := tx.Bucket(s.targetBucket)
		cursor := targetBucket.Cursor()
		for k, _ := cursor.First(); k != nil; k, _ = cursor.Next() {
			if _, keep := kept[string(k)]; !keep {
				err := cursor.Delete()
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
}
