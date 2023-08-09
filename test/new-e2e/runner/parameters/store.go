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

// Store exported type should have comment or be unexported
type Store struct {
	vs valueStore
}

func newStore(vs valueStore) Store {
	return Store{
		vs: vs,
	}
}

// Get exported method should have comment or be unexported
func (s Store) Get(key StoreKey) (string, error) {
	return getAndConvert(s.vs, key, func(s string) (string, error) { return s, nil })
}

// GetWithDefault exported method should have comment or be unexported
func (s Store) GetWithDefault(key StoreKey, def string) (string, error) {
	return getWithDefault(key, s.Get, def)
}

// GetBool exported method should have comment or be unexported
func (s Store) GetBool(key StoreKey) (bool, error) {
	return getAndConvert(s.vs, key, strconv.ParseBool)
}

// GetBoolWithDefault exported method should have comment or be unexported
func (s Store) GetBoolWithDefault(key StoreKey, def bool) (bool, error) {
	return getWithDefault(key, s.GetBool, def)
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
