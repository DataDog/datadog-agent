// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"errors"
	"reflect"
)

// BiMap provides a bidirectional map of keys and values.
type BiMap struct {
	key2Val reflect.Value
	val2Key reflect.Value
	keyType reflect.Type
	valType reflect.Type
}

// NewBiMap instantiates BiMap
func NewBiMap(k, v interface{}) *BiMap {
	ktype := reflect.TypeOf(k)
	vtype := reflect.TypeOf(v)
	bimap := &BiMap{
		keyType: ktype,
		valType: vtype,
		key2Val: reflect.MakeMap(reflect.MapOf(ktype, vtype)),
		val2Key: reflect.MakeMap(reflect.MapOf(vtype, ktype)),
	}
	return bimap
}

// GetKV gets value provided the key.
func (b *BiMap) GetKV(key interface{}) (interface{}, error) {
	keyType := reflect.TypeOf(key)
	if b.keyType != keyType {
		return nil, errors.New("bad key type provided for lookup")
	}

	v := b.key2Val.MapIndex(reflect.ValueOf(key))
	if v != reflect.ValueOf(nil) {
		return v.Interface(), nil
	}

	return nil, errors.New("no value found in lookup for key provided")
}

// GetKVReverse gets key provided the value.
func (b *BiMap) GetKVReverse(key interface{}) (interface{}, error) {
	keyType := reflect.TypeOf(key)
	if b.valType != keyType {
		return nil, errors.New("bad key type provided for lookup")
	}

	v := b.val2Key.MapIndex(reflect.ValueOf(key))
	if v != reflect.ValueOf(nil) {
		return v.Interface(), nil
	}

	return nil, errors.New("no key found in reverse lookup for value provided")
}

/*
GetKVBimap looks for the provided key both for keys and values in the map.

	The first occurrence will be returned.
*/
func (b *BiMap) GetKVBimap(key interface{}) (interface{}, error) {
	keyType := reflect.TypeOf(key)
	if b.keyType != keyType && b.valType != keyType {
		return nil, errors.New("bad type provided for bimap lookup")
	}

	if b.keyType == keyType {
		v := b.key2Val.MapIndex(reflect.ValueOf(key))
		if v != reflect.ValueOf(nil) {
			return v.Interface(), nil
		}
	}

	if b.valType == keyType {
		v := b.val2Key.MapIndex(reflect.ValueOf(key))
		if v != reflect.ValueOf(nil) {
			return v.Interface(), nil
		}
	}

	return nil, nil
}

// AddKV adds value `v` to map the map indexed with key `k`.
func (b *BiMap) AddKV(k, v interface{}) error {
	keyType := reflect.TypeOf(k)
	valType := reflect.TypeOf(v)
	if b.keyType != keyType && b.valType != valType {
		return errors.New("bad type provided for bimap insert")
	}

	b.key2Val.SetMapIndex(reflect.ValueOf(k), reflect.ValueOf(v))
	b.val2Key.SetMapIndex(reflect.ValueOf(v), reflect.ValueOf(k))

	return nil
}

// Keys returns a slice with all keys in the map.
func (b *BiMap) Keys() []interface{} {
	keys := b.key2Val.MapKeys()

	var ks []interface{}
	for _, k := range keys {
		ks = append(ks, k.Interface())
	}

	return ks
}

// Values returns a slice with all values in the map.
func (b *BiMap) Values() []interface{} {
	vals := b.val2Key.MapKeys()

	var vs []interface{}
	for _, v := range vals {
		vs = append(vs, v.Interface())
	}

	return vs
}
