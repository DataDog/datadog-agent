// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parameters

var _ valueStore = &mockStore{}

// MockStore dummy store meant for unit tests
// Its `get` always return the string value of the looked up key
type mockStore struct {
	store map[StoreKey]string
}

func NewMockStore(storeMap map[StoreKey]string) mockStore {
	return mockStore{store: storeMap}
}

func (ms mockStore) get(key StoreKey) (string, error) {
	if value, found := ms.store[key]; found {
		return value, nil
	}
	return string(key), nil
}
