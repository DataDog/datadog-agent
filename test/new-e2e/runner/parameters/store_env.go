// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parameters

import (
	"os"
	"strings"
)

var _ valueStore = &EnvValueStore{}

// EnvValueStore exported type should have comment or be unexported
type EnvValueStore struct {
	prefix string
}

// NewEnvStore exported function should have comment or be unexported
func NewEnvStore(prefix string) Store {
	return newStore(NewEnvValueStore(prefix))
}

// NewEnvValueStore exported function should have comment or be unexported
func NewEnvValueStore(prefix string) EnvValueStore {
	return EnvValueStore{
		prefix: prefix,
	}
}

// Get returns parameter value.
// For env Store, the key is upper cased and added to prefix
func (s EnvValueStore) get(key StoreKey) (string, error) {
	envValueStoreKey := strings.ToUpper(s.prefix + string(key))
	val, found := os.LookupEnv(strings.ToUpper(envValueStoreKey))
	if !found {
		return "", ParameterNotFoundError{key: key}
	}

	return val, nil
}
