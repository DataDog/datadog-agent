// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// ErrNotFound is an error for when a key is not found
var ErrNotFound = fmt.Errorf("not found")

// NewNode constructs a Node from either a map, a slice, or a scalar value
func NewNode(v interface{}, source model.Source) (Node, error) {
	switch it := v.(type) {
	case map[interface{}]interface{}:
		return newInnerNodeImpl(mapInterfaceToMapString(it), source)
	case map[string]interface{}:
		return newInnerNodeImpl(it, source)
	case []interface{}:
		return newArrayNodeImpl(it, source)
	}
	if isScalar(v) {
		return newLeafNodeImpl(v, source)
	}
	// Finally, try determining node type using reflection, should only be needed for unit tests that
	// supply data that isn't one of the "plain" types produced by parsing json, yaml, etc
	node, err := asReflectionNode(v)
	if err == errUnknownConversion {
		return nil, fmt.Errorf("could not create node from: %v of type %T", v, v)
	}
	return node, err
}

// Node represents an arbitrary node
type Node interface {
	GetChild(string) (Node, error)
	ChildrenKeys() ([]string, error)
	Set([]string, interface{}, model.Source) (bool, error)
}

// LeafNode represents a leaf node of the config
type LeafNode interface {
	GetAny() (interface{}, error)
	GetBool() (bool, error)
	GetInt() (int, error)
	GetFloat() (float64, error)
	GetString() (string, error)
	GetTime() (time.Time, error)
	GetDuration() (time.Duration, error)
	SetWithSource(interface{}, model.Source) error
	Source() model.Source
}
