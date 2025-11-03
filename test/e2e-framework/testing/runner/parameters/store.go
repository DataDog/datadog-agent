// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parameters

import (
	"errors"
	"strconv"
)

type valueStore interface {
	get(key StoreKey) (string, error)
}

// Store instance contains a valueStore
type Store struct {
	vs valueStore
}

func newStore(vs valueStore) Store {
	return Store{
		vs: vs,
	}
}

// Get returns a string value from the store
func (s Store) Get(key StoreKey) (string, error) {
	return getAndConvert(s.vs, key, func(s string) (string, error) { return s, nil })
}

// GetWithDefault returns a string value from the store with default on missing key
func (s Store) GetWithDefault(key StoreKey, def string) (string, error) {
	return getWithDefault(key, s.Get, def)
}

// GetBool returns a boolean value from the store
func (s Store) GetBool(key StoreKey) (bool, error) {
	return getAndConvert(s.vs, key, strconv.ParseBool)
}

// GetBoolWithDefault returns a boolean value from the store with default on missing key
func (s Store) GetBoolWithDefault(key StoreKey, def bool) (bool, error) {
	return getWithDefault(key, s.GetBool, def)
}

// GetInt returns an integer value from the store
func (s Store) GetInt(key StoreKey) (int, error) {
	return getAndConvert(s.vs, key, strconv.Atoi)
}

// GetIntWithDefault returns an integer value from the store with default on missing key
func (s Store) GetIntWithDefault(key StoreKey, def int) (int, error) {
	return getWithDefault(key, s.GetInt, def)
}

func getWithDefault[T any](key StoreKey, getFunc func(StoreKey) (T, error), defaultValue T) (T, error) {
	val, err := getFunc(key)
	if err != nil {
		if errors.As(err, &ParameterNotFoundError{}) {
			return defaultValue, nil
		}

		return val, err
	}

	return val, nil
}

func getAndConvert[T any](vs valueStore, key StoreKey, convFunc func(string) (T, error)) (T, error) {
	var res T

	v, err := vs.get(key)
	if err != nil {
		return res, err
	}

	res, err = convFunc(v)
	if err != nil {
		return res, err
	}

	return res, nil
}
