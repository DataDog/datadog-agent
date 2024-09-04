// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package structure defines a helper to retrieve structured data from the config
package structure

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// UnmarshalKey retrieves data from the config at the given key and deserializes it
// to be stored on the target struct. It is implemented entirely using reflection, and
// does not depend upon details of the data model of the config.
// Target struct can use of struct tag of "yaml", "json", or "mapstructure" to rename fields
func UnmarshalKey(cfg model.Reader, key string, target interface{}) error {
	source, err := newNode(reflect.ValueOf(cfg.Get(key)))
	if err != nil {
		return err
	}
	outValue := reflect.ValueOf(target)
	if outValue.Kind() == reflect.Pointer {
		outValue = reflect.Indirect(outValue)
	}
	switch outValue.Kind() {
	case reflect.Map:
		return copyMap(outValue, source)
	case reflect.Struct:
		return copyStruct(outValue, source)
	case reflect.Slice:
		if arr, ok := source.(arrayNode); ok {
			return copyList(outValue, arr)
		}
		return fmt.Errorf("can not UnmarshalKey to a slice from a non-list source")
	default:
		return fmt.Errorf("can only UnmarshalKey to struct, map, or slice, got %v", outValue.Kind())
	}
}

var errNotFound = fmt.Errorf("not found")

// leafNode represents a leaf with a scalar value

type leafNode interface {
	GetBool() (bool, error)
	GetInt() (int, error)
	GetFloat() (float64, error)
	GetString() (string, error)
}

type leafNodeImpl struct {
	// val must be a scalar kind
	val reflect.Value
}

var _ leafNode = (*leafNodeImpl)(nil)
var _ node = (*leafNodeImpl)(nil)

// arrayNode represents a node with an ordered array of children

type arrayNode interface {
	Size() int
	Index(int) (node, error)
}

type arrayNodeImpl struct {
	// val must be a Slice with Len() and Index()
	val reflect.Value
}

var _ arrayNode = (*arrayNodeImpl)(nil)
var _ node = (*arrayNodeImpl)(nil)

// node represents an arbitrary node of the tree

type node interface {
	GetChild(string) (node, error)
	ChildrenKeys() ([]string, error)
}

type innerNodeImpl struct {
	// val must be a struct
	val reflect.Value
}

type innerMapNodeImpl struct {
	// val must be a map[string]interface{}
	val reflect.Value
}

var _ node = (*innerNodeImpl)(nil)
var _ node = (*innerMapNodeImpl)(nil)

// all nodes, leaf, inner, and array nodes, each act as nodes
func newNode(v reflect.Value) (node, error) {
	if v.Kind() == reflect.Struct {
		return &innerNodeImpl{val: v}, nil
	} else if v.Kind() == reflect.Map {
		return &innerMapNodeImpl{val: v}, nil
	} else if v.Kind() == reflect.Slice {
		return &arrayNodeImpl{val: v}, nil
	} else if isScalarKind(v) {
		return &leafNodeImpl{val: v}, nil
	}
	return nil, fmt.Errorf("could not create node from: %v of type %T and kind %v", v, v, v.Kind())
}

// GetChild returns the child node at the given key, or an error if not found
func (n *innerNodeImpl) GetChild(key string) (node, error) {
	findex := findFieldMatch(n.val, key)
	if findex == -1 {
		return nil, errNotFound
	}
	inner := n.val.Field(findex)
	if inner.Kind() == reflect.Interface {
		inner = inner.Elem()
	}
	return newNode(inner)
}

// ChildrenKeys returns the list of keys of the children of the given node, if it is a map
func (n *innerNodeImpl) ChildrenKeys() ([]string, error) {
	structType := n.val.Type()
	keys := make([]string, 0, n.val.NumField())
	for i := 0; i < structType.NumField(); i++ {
		f := structType.Field(i)
		ch, _ := utf8.DecodeRuneInString(f.Name)
		if unicode.IsLower(ch) {
			continue
		}
		keys = append(keys, fieldNameToKey(f))
	}
	return keys, nil
}

// GetChild returns the child node at the given key, or an error if not found
func (n *innerMapNodeImpl) GetChild(key string) (node, error) {
	inner := n.val.MapIndex(reflect.ValueOf(key))
	if !inner.IsValid() {
		return nil, errNotFound
	}
	if inner.Kind() == reflect.Interface {
		inner = inner.Elem()
	}
	return newNode(inner)
}

// ChildrenKeys returns the list of keys of the children of the given node, if it is a map
func (n *innerMapNodeImpl) ChildrenKeys() ([]string, error) {
	mapkeys := n.val.MapKeys()
	keys := make([]string, 0, len(mapkeys))
	for _, kv := range mapkeys {
		if kstr, ok := kv.Interface().(string); ok {
			keys = append(keys, kstr)
		} else {
			return nil, fmt.Errorf("map node has invalid non-string key: %v", kv)
		}
	}
	return keys, nil
}

// GetChild returns an error because array node does not have children accessible by name
func (n *arrayNodeImpl) GetChild(string) (node, error) {
	return nil, fmt.Errorf("arrayNodeImpl.GetChild not implemented")
}

// ChildrenKeys returns an error because array node does not have children accessible by name
func (n *arrayNodeImpl) ChildrenKeys() ([]string, error) {
	return nil, fmt.Errorf("arrayNodeImpl.ChildrenKeys not implemented")
}

// Size returns number of children in the list
func (n *arrayNodeImpl) Size() int {
	return n.val.Len()
}

// Index returns the kth element of the list
func (n *arrayNodeImpl) Index(k int) (node, error) {
	// arrayNodeImpl assumes val is an Array with Len() and Index()
	elem := n.val.Index(k)
	if elem.Kind() == reflect.Interface {
		elem = elem.Elem()
	}
	return newNode(elem)
}

// GetChild returns an error because a leaf has no children
func (n *leafNodeImpl) GetChild(string) (node, error) {
	return nil, fmt.Errorf("can't GetChild of a leaf node")
}

// ChildrenKeys returns an error because a leaf has no children
func (n *leafNodeImpl) ChildrenKeys() ([]string, error) {
	return nil, fmt.Errorf("can't get ChildrenKeys of a leaf node")
}

// GetBool returns the scalar as a bool, or an error otherwise
func (n *leafNodeImpl) GetBool() (bool, error) {
	if n.val.Kind() == reflect.Bool {
		return n.val.Bool(), nil
	} else if n.val.Kind() == reflect.String {
		return convertToBool(n.val.String())
	}
	return false, newConversionError(n.val, "bool")
}

// GetInt returns the scalar as a int, or an error otherwise
func (n *leafNodeImpl) GetInt() (int, error) {
	switch n.val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(n.val.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int(n.val.Uint()), nil
	case reflect.Float32, reflect.Float64:
		return int(n.val.Float()), nil
	}
	return 0, newConversionError(n.val, "int")
}

// GetFloat returns the scalar as a float64, or an error otherwise
func (n *leafNodeImpl) GetFloat() (float64, error) {
	switch n.val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(n.val.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(n.val.Uint()), nil
	case reflect.Float32, reflect.Float64:
		return float64(n.val.Float()), nil
	}
	return 0, newConversionError(n.val, "float")
}

// GetString returns the scalar as a string, or an error otherwise
func (n *leafNodeImpl) GetString() (string, error) {
	if n.val.Kind() == reflect.String {
		return n.val.String(), nil
	}
	return "", newConversionError(n.val, "string")
}

// convert a string to a bool using standard yaml constants
func convertToBool(text string) (bool, error) {
	lower := strings.ToLower(text)
	if lower == "y" || lower == "yes" || lower == "on" || lower == "true" {
		return true, nil
	} else if lower == "n" || lower == "no" || lower == "off" || lower == "false" {
		return false, nil
	}
	return false, newConversionError(reflect.ValueOf(text), "bool")
}

func fieldNameToKey(field reflect.StructField) string {
	name := field.Name
	if tagtext := field.Tag.Get("yaml"); tagtext != "" {
		name = tagtext
	} else if tagtext := field.Tag.Get("json"); tagtext != "" {
		name = tagtext
	} else if tagtext := field.Tag.Get("mapstructure"); tagtext != "" {
		name = tagtext
	}
	// skip any additional specifiers such as ",omitempty"
	if commaPos := strings.IndexRune(name, ','); commaPos != -1 {
		name = name[:commaPos]
	}
	return name
}

func copyStruct(target reflect.Value, source node) error {
	targetType := target.Type()
	for i := 0; i < targetType.NumField(); i++ {
		f := targetType.Field(i)
		ch, _ := utf8.DecodeRuneInString(f.Name)
		if unicode.IsLower(ch) {
			continue
		}
		child, err := source.GetChild(fieldNameToKey(f))
		if err == errNotFound {
			continue
		} else if err != nil {
			return err
		}
		err = copyAny(target.FieldByName(f.Name), child)
		if err != nil {
			return err
		}
	}
	return nil
}

func copyMap(target reflect.Value, source node) error {
	// TODO: Should handle maps with more complex types in a future PR
	ktype := reflect.TypeOf("")
	vtype := reflect.TypeOf("")
	mtype := reflect.MapOf(ktype, vtype)
	results := reflect.MakeMap(mtype)

	mapKeys, err := source.ChildrenKeys()
	if err != nil {
		return err
	}
	for _, mkey := range mapKeys {
		child, err := source.GetChild(mkey)
		if err != nil {
			return err
		}
		if child == nil {
			continue
		}
		if scalar, ok := child.(leafNode); ok {
			if mval, err := scalar.GetString(); err == nil {
				results.SetMapIndex(reflect.ValueOf(mkey), reflect.ValueOf(mval))
			} else {
				return fmt.Errorf("TODO: only map[string]string supported currently")
			}
		}
	}
	target.Set(results)
	return nil
}

func copyLeaf(target reflect.Value, source leafNode) error {
	if source == nil {
		return fmt.Errorf("source value is not a scalar")
	}
	switch target.Kind() {
	case reflect.Bool:
		v, err := source.GetBool()
		if err != nil {
			return err
		}
		target.SetBool(v)
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v, err := source.GetInt()
		if err != nil {
			return err
		}
		target.SetInt(int64(v))
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v, err := source.GetInt()
		if err != nil {
			return err
		}
		target.SetUint(uint64(v))
		return nil
	case reflect.Float32, reflect.Float64:
		v, err := source.GetFloat()
		if err != nil {
			return err
		}
		target.SetFloat(float64(v))
		return nil
	case reflect.String:
		v, err := source.GetString()
		if err != nil {
			return err
		}
		target.SetString(v)
		return nil
	}
	return fmt.Errorf("unsupported scalar type %v", target.Kind())
}

func copyList(target reflect.Value, source arrayNode) error {
	if source == nil {
		return fmt.Errorf("source value is not a list")
	}
	elemType := target.Type()
	elemType = elemType.Elem()
	numElems := source.Size()
	results := reflect.MakeSlice(reflect.SliceOf(elemType), numElems, numElems)
	for k := 0; k < numElems; k++ {
		elemSource, err := source.Index(k)
		if err != nil {
			return err
		}
		ptrOut := reflect.New(elemType)
		outTarget := ptrOut.Elem()
		err = copyAny(outTarget, elemSource)
		if err != nil {
			return err
		}
		results.Index(k).Set(outTarget)
	}
	target.Set(results)
	return nil
}

func copyAny(target reflect.Value, source node) error {
	if target.Kind() == reflect.Pointer {
		allocPtr := reflect.New(target.Type().Elem())
		target.Set(allocPtr)
		target = allocPtr.Elem()
	}
	if isScalarKind(target) {
		if leaf, ok := source.(leafNode); ok {
			return copyLeaf(target, leaf)
		}
		return fmt.Errorf("can't copy into target: scalar required, but source is not a leaf")
	} else if target.Kind() == reflect.Map {
		return copyMap(target, source)
	} else if target.Kind() == reflect.Struct {
		return copyStruct(target, source)
	} else if target.Kind() == reflect.Slice {
		if arr, ok := source.(arrayNode); ok {
			return copyList(target, arr)
		}
		return fmt.Errorf("can't copy into target: []T required, but source is not an array")
	} else if target.Kind() == reflect.Invalid {
		return fmt.Errorf("can't copy invalid value %s : %v", target, target.Kind())
	}
	return fmt.Errorf("unknown value to copy: %v", target.Type())
}

func isScalarKind(v reflect.Value) bool {
	k := v.Kind()
	return (k >= reflect.Bool && k <= reflect.Float64) || k == reflect.String
}

func findFieldMatch(val reflect.Value, key string) int {
	schema := val.Type()
	for i := 0; i < schema.NumField(); i++ {
		if key == fieldNameToKey(schema.Field(i)) {
			return i
		}
	}
	return -1
}

func newConversionError(v reflect.Value, expectType string) error {
	return fmt.Errorf("could not convert to %s: %v of type %T and Kind %v", expectType, v, v, v.Kind())
}
