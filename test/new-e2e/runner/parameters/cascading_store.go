// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parameters

import (
	"errors"
)

var _ valueStore = &CascadingValueStore{}

// CascadingValueStore exported type should have comment or be unexported
type CascadingValueStore struct {
	valueStores []valueStore
}

// NewCascadingStore exported function should have comment or be unexported
func NewCascadingStore(valueStores ...valueStore) Store {
	return newStore(CascadingValueStore{
		valueStores: valueStores,
	})
}

func (s CascadingValueStore) get(key StoreKey) (string, error) {
	for _, valueStore := range s.valueStores {
		v, err := valueStore.get(key)

		if err == nil {
			return v, nil
		}
		if !errors.As(err, &ParameterNotFoundError{}) {
			return "", err
		}
	}
	return "", ParameterNotFoundError{key: key}
}
