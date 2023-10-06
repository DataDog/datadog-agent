// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parameters

import (
	"os"
	"strings"
)

var _ valueStore = &envValueStore{}

type envValueStore struct {
	prefix string
}

// NewEnvStore creates a new store based on environment variables
func NewEnvStore(prefix string) Store {
	return newStore(newEnvValueStore(prefix))
}

func newEnvValueStore(prefix string) envValueStore {
	return envValueStore{
		prefix: prefix,
	}
}

// Get returns parameter value.
// For env Store, the key is upper cased and added to prefix
func (s envValueStore) get(key StoreKey) (string, error) {
	envValueStoreKey := strings.ToUpper(s.prefix + string(key))
	val, found := os.LookupEnv(strings.ToUpper(envValueStoreKey))
	if !found {
		return "", ParameterNotFoundError{key: key}
	}

	return val, nil
}
