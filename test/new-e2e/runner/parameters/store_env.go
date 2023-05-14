// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parameters

import (
	"os"
	"strings"
)

type envValueStore struct {
	prefix string
}

func NewEnvStore(prefix string) Store {
	return newStore(NewEnvValueStore(prefix))
}

func NewEnvValueStore(prefix string) envValueStore {
	return envValueStore{
		prefix: prefix,
	}
}

// Get returns parameter value.
// For env Store, the key is upper cased and added to prefix
func (s envValueStore) get(key string) (string, error) {
	key = strings.ToUpper(s.prefix + key)
	val, found := os.LookupEnv(strings.ToUpper(key))
	if !found {
		return "", ParameterNotFoundError{key: key}
	}

	return val, nil
}
