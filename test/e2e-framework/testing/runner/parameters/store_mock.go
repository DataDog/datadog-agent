// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package parameters

var _ valueStore = &mockStore{}

// MockStore dummy store meant for unit tests
// Its `get` always return the string value of the looked up key
type mockStore struct {
	values map[StoreKey]string
}

// NewMockStore creates a new mock store
func NewMockStore(values map[StoreKey]string) Store {
	return newStore(mockStore{values: values})
}

func (ms mockStore) get(key StoreKey) (string, error) {
	if value, found := ms.values[key]; found {
		return value, nil
	}
	return string(key), nil
}
