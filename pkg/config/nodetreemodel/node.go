// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"golang.org/x/exp/maps"
)

// ErrNotFound is an error for when a key is not found
var ErrNotFound = fmt.Errorf("not found")

// NewNode constructs a Node from either a map, a slice, or a scalar value
func NewNode(v interface{}) (Node, error) {
	switch it := v.(type) {
	case map[interface{}]interface{}:
		return newMapNodeImpl(mapInterfaceToMapString(it))
	case map[string]interface{}:
		return newMapNodeImpl(it)
	case []interface{}:
		return newArrayNodeImpl(it)
	}
	if isScalar(v) {
		return newLeafNodeImpl(v)
	}
	// Finally, try determining node type using reflection, should only be needed for unit tests that
	// supply data that isn't one of the "plain" types produced by parsing json, yaml, etc
	node, err := asReflectionNode(v)
	if err == errUnknownConversion {
		return nil, fmt.Errorf("could not create node from: %v of type %T", v, v)
	}
	return node, err
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
}

// ArrayNode represents a node with ordered, numerically indexed set of children
type ArrayNode interface {
	Size() int
	Index(int) (Node, error)
}

// Node represents an arbitrary node
type Node interface {
	GetChild(string) (Node, error)
	ChildrenKeys() ([]string, error)
}

// leafNode represents a leaf with a scalar value

type leafNodeImpl struct {
	// val must be a scalar kind
	val    interface{}
	source model.Source
}

func newLeafNodeImpl(v interface{}) (Node, error) {
	if isScalar(v) {
		return &leafNodeImpl{val: v}, nil
	}
	return nil, fmt.Errorf("cannot create leaf node from %v of type %T", v, v)
}

var _ LeafNode = (*leafNodeImpl)(nil)
var _ Node = (*leafNodeImpl)(nil)

// arrayNode represents a node with an ordered array of children

type arrayNodeImpl struct {
	nodes []Node
}

func newArrayNodeImpl(v []interface{}) (Node, error) {
	nodes := make([]Node, 0, len(v))
	for _, it := range v {
		if n, ok := it.(Node); ok {
			nodes = append(nodes, n)
			continue
		}
		n, err := NewNode(it)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return &arrayNodeImpl{nodes: nodes}, nil
}

var _ ArrayNode = (*arrayNodeImpl)(nil)
var _ Node = (*arrayNodeImpl)(nil)

// node represents an arbitrary node of the tree

type mapNodeImpl struct {
	val map[string]interface{}
	// remapCase maps each lower-case key to the original case. This
	// enables GetChild to retrieve values using case-insensitive keys
	remapCase map[string]string
}

func newMapNodeImpl(v map[string]interface{}) (Node, error) {
	return &mapNodeImpl{val: v, remapCase: makeRemapCase(v)}, nil
}

var _ Node = (*mapNodeImpl)(nil)

///////

func isScalar(v interface{}) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	case bool, string, float32, float64, time.Time, time.Duration:
		return true
	default:
		return false
	}
}

// creates a map that converts keys from their lower-cased version to their original case
func makeRemapCase(m map[string]interface{}) map[string]string {
	remap := make(map[string]string)
	for k := range m {
		remap[strings.ToLower(k)] = k
	}
	return remap
}

func mapInterfaceToMapString(m map[interface{}]interface{}) map[string]interface{} {
	res := make(map[string]interface{}, len(m))
	for k, v := range m {
		mk := ""
		if str, ok := k.(string); ok {
			mk = str
		} else {
			mk = fmt.Sprintf("%s", k)
		}
		res[mk] = v
	}
	return res
}

/////

// GetChild returns the child node at the given case-insensitive key, or an error if not found
func (n *mapNodeImpl) GetChild(key string) (Node, error) {
	mkey := n.remapCase[strings.ToLower(key)]
	child, found := n.val[mkey]
	if !found {
		return nil, ErrNotFound
	}
	// If the map is already storing a Node, return it
	if n, ok := child.(Node); ok {
		return n, nil
	}
	// Otherwise construct a new node
	return NewNode(child)
}

// ChildrenKeys returns the list of keys of the children of the given node, if it is a map
func (n *mapNodeImpl) ChildrenKeys() ([]string, error) {
	mapkeys := maps.Keys(n.val)
	// map keys are iterated non-deterministically, sort them
	slices.Sort(mapkeys)
	return mapkeys, nil
}

// GetChild returns an error because array node does not have children accessible by name
func (n *arrayNodeImpl) GetChild(string) (Node, error) {
	return nil, fmt.Errorf("arrayNodeImpl.GetChild not implemented")
}

// ChildrenKeys returns an error because array node does not have children accessible by name
func (n *arrayNodeImpl) ChildrenKeys() ([]string, error) {
	return nil, fmt.Errorf("arrayNodeImpl.ChildrenKeys not implemented")
}

// Size returns number of children in the list
func (n *arrayNodeImpl) Size() int {
	return len(n.nodes)
}

// Index returns the kth element of the list
func (n *arrayNodeImpl) Index(k int) (Node, error) {
	if k < 0 || k >= len(n.nodes) {
		return nil, ErrNotFound
	}
	return n.nodes[k], nil
}

// GetChild returns an error because a leaf has no children
func (n *leafNodeImpl) GetChild(key string) (Node, error) {
	return nil, fmt.Errorf("can't GetChild(%s) of a leaf node", key)
}

// ChildrenKeys returns an error because a leaf has no children
func (n *leafNodeImpl) ChildrenKeys() ([]string, error) {
	return nil, fmt.Errorf("can't get ChildrenKeys of a leaf node")
}

// GetAny returns the scalar as an interface
func (n *leafNodeImpl) GetAny() (interface{}, error) {
	return n, nil
}

// GetBool returns the scalar as a bool, or an error otherwise
func (n *leafNodeImpl) GetBool() (bool, error) {
	switch it := n.val.(type) {
	case bool:
		return it, nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		num, err := n.GetInt()
		if err != nil {
			return false, err
		}
		return num != 0, nil
	case string:
		return convertToBool(it)
	default:
		return false, newConversionError(n, "bool")
	}
}

// GetInt returns the scalar as a int, or an error otherwise
func (n *leafNodeImpl) GetInt() (int, error) {
	switch it := n.val.(type) {
	case int:
		return int(it), nil
	case int8:
		return int(it), nil
	case int16:
		return int(it), nil
	case int32:
		return int(it), nil
	case int64:
		return int(it), nil
	case uint:
		return int(it), nil
	case uint8:
		return int(it), nil
	case uint16:
		return int(it), nil
	case uint32:
		return int(it), nil
	case uint64:
		return int(it), nil
	case float32:
		return int(it), nil
	case float64:
		return int(it), nil
	case time.Duration:
		return int(it), nil
	}
	return 0, newConversionError(n.val, "int")
}

// GetFloat returns the scalar as a float64, or an error otherwise
func (n *leafNodeImpl) GetFloat() (float64, error) {
	switch it := n.val.(type) {
	case int:
		return float64(it), nil
	case int8:
		return float64(it), nil
	case int16:
		return float64(it), nil
	case int32:
		return float64(it), nil
	case int64:
		return float64(it), nil
	case uint:
		return float64(it), nil
	case uint8:
		return float64(it), nil
	case uint16:
		return float64(it), nil
	case uint32:
		return float64(it), nil
	case uint64:
		return float64(it), nil
	case float32:
		return float64(it), nil
	case float64:
		return float64(it), nil
	}
	return 0, newConversionError(n.val, "float")
}

// GetString returns the scalar as a string, or an error otherwise
func (n *leafNodeImpl) GetString() (string, error) {
	switch it := n.val.(type) {
	case int, int8, int16, int32, int64:
		num, err := n.GetInt()
		if err != nil {
			return "", err
		}
		stringVal := strconv.FormatInt(int64(num), 10)
		return stringVal, nil
	case uint, uint8, uint16, uint32, uint64:
		num, err := n.GetInt()
		if err != nil {
			return "", err
		}
		stringVal := strconv.FormatUint(uint64(num), 10)
		return stringVal, nil
	case float32:
		f, err := n.GetFloat()
		if err != nil {
			return "", err
		}
		stringVal := strconv.FormatFloat(f, 'f', -1, 32)
		return stringVal, nil
	case float64:
		f, err := n.GetFloat()
		if err != nil {
			return "", err
		}
		stringVal := strconv.FormatFloat(f, 'f', -1, 64)
		return stringVal, nil
	case string:
		return it, nil
	}
	return "", newConversionError(n.val, "string")
}

// GetTime returns the scalar as a time, or an error otherwise, not implemented
func (n *leafNodeImpl) GetTime() (time.Time, error) {
	return time.Time{}, fmt.Errorf("not implemented")
}

// GetDuration returns the scalar as a duration, or an error otherwise, not implemented
func (n *leafNodeImpl) GetDuration() (time.Duration, error) {
	return time.Duration(0), fmt.Errorf("not implemented")
}

// Set assigns a value in the config, for the given source
func (n *leafNodeImpl) SetWithSource(newValue interface{}, source model.Source) error {
	// TODO: enforce type-checking, return an error if type changes
	n.val = newValue
	n.source = source
	// TODO: Record previous value and source
	return nil
}

// convert a string to a bool using standard yaml constants
func convertToBool(text string) (bool, error) {
	lower := strings.ToLower(text)
	if lower == "y" || lower == "yes" || lower == "on" || lower == "true" || lower == "1" {
		return true, nil
	} else if lower == "n" || lower == "no" || lower == "off" || lower == "false" || lower == "0" {
		return false, nil
	}
	return false, newConversionError(text, "bool")
}

func newConversionError(v interface{}, expectType string) error {
	return fmt.Errorf("could not convert to %s: %v of type %T", expectType, v, v)
}
