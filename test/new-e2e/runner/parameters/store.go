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
	get(key string) (string, error)
}

type Store struct {
	vs valueStore
}

func newStore(vs valueStore) Store {
	return Store{
		vs: vs,
	}
}

func (s Store) Get(key string) (string, error) {
	return getAndConvert(s.vs, key, func(s string) (string, error) { return s, nil })
}

func (s Store) GetWithDefault(key, def string) (string, error) {
	return getWithDefault(key, s.Get, def)
}

func (s Store) GetBool(key string) (bool, error) {
	return getAndConvert(s.vs, key, strconv.ParseBool)
}

func (s Store) GetBoolWithDefault(key string, def bool) (bool, error) {
	return getWithDefault(key, s.GetBool, def)
}

func getWithDefault[T any](key string, getFunc func(string) (T, error), defaultValue T) (T, error) {
	val, err := getFunc(key)
	if err != nil {
		if errors.As(err, &ParameterNotFoundError{}) {
			return defaultValue, nil
		}

		return val, err
	}

	return val, nil
}

func getAndConvert[T any](vs valueStore, key string, convFunc func(string) (T, error)) (T, error) {
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
