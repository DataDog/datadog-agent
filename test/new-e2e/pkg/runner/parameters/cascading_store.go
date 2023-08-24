// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parameters

import (
	"errors"
)

var _ valueStore = &CascadingValueStore{}

// CascadingValueStore instance contains an array of valueStore
// ordered by priority.
// Parameters in a cascading value store are looked up iterating through
// all value stores and return at first match
type CascadingValueStore struct {
	stores []Store
}

// NewCascadingStore creates a new cascading store
func NewCascadingStore(stores ...Store) Store {
	return newStore(CascadingValueStore{
		stores: stores,
	})
}

func (s CascadingValueStore) get(key StoreKey) (string, error) {
	for _, store := range s.stores {
		v, err := store.Get(key)

		if err == nil {
			return v, nil
		}
		if !errors.As(err, &ParameterNotFoundError{}) {
			return "", err
		}
	}
	return "", ParameterNotFoundError{key: key}
}
