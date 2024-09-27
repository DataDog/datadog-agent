// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package uptane

import (
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// targetStore persists all the target files present in the current director targets.json
type targetStore struct {
	db           *transactionalStore
	targetBucket string
}

func newTargetStore(db *transactionalStore) *targetStore {
	return &targetStore{
		db:           db,
		targetBucket: "targets",
	}
}

func (s *targetStore) storeTargetFiles(targetFiles []*pbgo.File) error {
	return s.db.update(func(t *transaction) error {
		for _, target := range targetFiles {
			t.put(s.targetBucket, trimHashTargetPath(target.Path), target.Raw)
		}
		return nil
	})
}

func (s *targetStore) getTargetFile(path string) ([]byte, bool, error) {
	var target []byte
	var err error
	err = s.db.view(func(t *transaction) error {
		target, err = t.get(s.targetBucket, trimHashTargetPath(path))
		return err
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
	return s.db.update(func(t *transaction) error {
		return t.pruneTargetFiles(s.targetBucket, keptPaths)
	})
}
