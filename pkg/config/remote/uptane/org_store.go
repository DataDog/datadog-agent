// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package uptane

import (
	"fmt"
)

// orgStore persists org-specific data
type orgStore struct {
	db        *transactionalStore
	orgBucket string
}

func newOrgStore(db *transactionalStore) *orgStore {
	return &orgStore{
		db:        db,
		orgBucket: "org",
	}
}

func (s *orgStore) storeOrgUUID(rootVersion uint64, orgUUID string) error {
	return s.db.update(func(t *transaction) error {
		t.put(s.orgBucket, fmt.Sprintf("%v-uuid", rootVersion), []byte(orgUUID))
		return nil
	})
}

func (s *orgStore) getOrgUUID(rootVersion uint64) (string, bool, error) {
	var orgUUID []byte
	var err error
	err = s.db.view(func(t *transaction) error {
		orgUUID, err = t.get(s.orgBucket, fmt.Sprintf("%v-uuid", rootVersion))
		return err
	})
	if err != nil {
		return "", false, err
	}
	if len(orgUUID) == 0 {
		return "", false, nil
	}
	return string(orgUUID), true, nil
}
