// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/config/helper"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// ErrNotFound is an error for when a key is not found
var ErrNotFound = errors.New("not found")

func mapToMapString(m reflect.Value) map[string]interface{} {
	if v, ok := m.Interface().(map[string]interface{}); ok {
		// no need to convert the map
		return v
	}

	res := make(map[string]interface{}, m.Len())

	iter := m.MapRange()
	for iter.Next() {
		k := iter.Key()
		mk := ""
		if k.Kind() == reflect.String {
			mk = k.Interface().(string)
		} else {
			mk = fmt.Sprintf("%s", k.Interface())
		}
		res[mk] = iter.Value().Interface()
	}
	return res
}

// NewNodeTree will recursively create nodes from the input value to construct a tree
func NewNodeTree(v interface{}, source model.Source) (*nodeImpl, error) {
	if helper.IsNilValue(v) {
		// nil as a value acts as the zero value, and the cast library will correctly
		// convert it to zero values for the types we handle
		return newLeafNode(nil, source), nil
	}
	switch it := v.(type) {
	case []interface{}:
		return newLeafNode(it, source), nil
	}

	// handle all map types that can be converted to map[string]interface{}
	if v := reflect.ValueOf(v); v.Kind() == reflect.Map {
		children, err := makeChildNodeTrees(mapToMapString(v), source)
		if err != nil {
			return nil, err
		}
		return newInnerNode(children), nil
	}

	if isScalar(v) {
		return newLeafNode(v, source), nil
	}
	// Finally, try determining node type using reflection, should only be needed for unit tests that
	// supply data that isn't one of the "plain" types produced by parsing json, yaml, etc
	node, err := asReflectionNode(v)
	if err == errUnknownConversion {
		return nil, fmt.Errorf("could not create node from: %v of type %T", v, v)
	}
	return node, err
}

func makeChildNodeTrees(input map[string]interface{}, source model.Source) (map[string]*nodeImpl, error) {
	children := make(map[string]*nodeImpl)
	for k, v := range input {
		node, err := NewNodeTree(v, source)
		if err != nil {
			return nil, err
		}
		children[k] = node
	}
	return children, nil
}

// Node is a inner or leaf node in the config tree
type Node interface {
	IsLeafNode() bool
	IsInnerNode() bool
	Get() interface{}
	ChildrenKeys() []string
}

type nodeImpl struct {
	children map[string]*nodeImpl
	val      interface{}
	source   model.Source
}

var _ Node = (*nodeImpl)(nil)
