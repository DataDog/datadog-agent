// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package structure defines a helper to retrieve structured data from the config
package structure

import (
	"fmt"
	"reflect"
	"unicode"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// UnmarshalKey retrieves data from the config at the given key and deserializes it
// to be stored on the target struct. It is implemented entirely using reflection, and
// does not depend upon details of the data model of the config.
// Target struct can use of struct tag of "yaml", "json", or "mapstructure" to rename fields
func UnmarshalKey(cfg model.Reader, key string, target interface{}) error {
	source := newNode(reflect.ValueOf(cfg.Get(key)))
	outValue := reflect.ValueOf(target)
	if outValue.Kind() == reflect.Pointer {
		outValue = reflect.Indirect(outValue)
	}
	if outValue.Kind() == reflect.Map {
		return copyMap(outValue, source)
	}
	if outValue.Kind() == reflect.Struct {
		return copyStruct(outValue, source)
	}
	if outValue.Kind() == reflect.Slice {
		return copyList(outValue, source.AsList())
	}
	return fmt.Errorf("can only UnmarshalKey to struct or slice, got %v", outValue.Kind())
}

type nodeType int

const (
	invalidNodeType nodeType = iota
	scalarNodeType
	listNodeType
	mapNodeType
	structNodeType
)

type leafimpl struct {
	val reflect.Value
}

type leaf interface {
	GetBool() (bool, error)
	GetInt() (int, error)
	GetString() (string, error)
}

type nodeListImpl struct {
	val reflect.Value
}

type nodeList interface {
	Size() int
	Index(int) node
}

type nodeimpl struct {
	typ nodeType
	val reflect.Value
}

type node interface {
	GetChild(string) node
	ChildrenKeys() []string
	AsList() nodeList
	AsScalar() leaf
}

func newNode(v reflect.Value) node {
	if v.Kind() == reflect.Struct {
		return &nodeimpl{typ: structNodeType, val: v}
	}
	if v.Kind() == reflect.Map {
		return &nodeimpl{typ: mapNodeType, val: v}
	}
	if v.Kind() == reflect.Slice {
		return &nodeimpl{typ: listNodeType, val: v}
	}
	if isScalar(v) {
		return &nodeimpl{typ: scalarNodeType, val: v}
	}
	panic(fmt.Errorf("could not create node from: %v of type %T and kind %v", v, v, v.Kind()))
}

// GetChild returns the child node at the given key, or nil if not found
func (n *nodeimpl) GetChild(key string) node {
	if n.typ != mapNodeType && n.typ != structNodeType {
		return nil
	}
	if n.val.Kind() == reflect.Map {
		inner := n.val.MapIndex(reflect.ValueOf(key))
		if !inner.IsValid() {
			return nil
		}
		if inner.IsNil() {
			return nil
		}
		if inner.Kind() == reflect.Interface {
			inner = inner.Elem()
		}
		return newNode(inner)
	}
	if n.val.Kind() == reflect.Struct {
		findex := findFieldMatch(n.val, key)
		if findex == -1 {
			return nil
		}
		inner := n.val.Field(findex)
		if !inner.IsValid() {
			return nil
		}
		if inner.Kind() == reflect.Interface {
			inner = inner.Elem()
		}
		return newNode(inner)
	}
	return nil
}

// ChildrenKeys returns the list of keys of the children of the given node
func (n *nodeimpl) ChildrenKeys() []string {
	if n.val.Kind() == reflect.Map {
		keyvals := n.val.MapKeys()
		keys := make([]string, 0, len(keyvals))
		for _, kv := range keyvals {
			kinf := kv.Interface()
			kstr, ok := kinf.(string)
			if ok {
				keys = append(keys, kstr)
			}
		}
		return keys
	}
	return nil
}

// AsScalar returns the node as a leaf if possible, otherwise nil
func (n *nodeimpl) AsScalar() leaf {
	if n.typ != scalarNodeType {
		return nil
	}
	return &leafimpl{val: n.val}
}

// AsList returns the node as a list if possible, otherwise nil
func (n *nodeimpl) AsList() nodeList {
	if n.typ != listNodeType {
		return nil
	}
	return &nodeListImpl{val: n.val}
}

// Size returns the number of elements in the list
func (n *nodeListImpl) Size() int {
	return n.val.Len()
}

// Index returns the kth element of the list
func (n *nodeListImpl) Index(k int) node {
	elem := n.val.Index(k)
	if elem.Kind() == reflect.Interface {
		elem = elem.Elem()
	}
	return newNode(elem)
}

// GetBool returns the scalar as a bool, or an error otherwise
func (n *leafimpl) GetBool() (bool, error) {
	if n.val.Kind() == reflect.Bool {
		return n.val.Bool(), nil
	}
	if n.val.Kind() == reflect.String {
		if n.val.String() == "true" {
			return true, nil
		} else if n.val.String() == "false" {
			return false, nil
		}
	}
	return false, conversionError(n.val, "bool")
}

// GetInt returns the scalar as a int, or an error otherwise
func (n *leafimpl) GetInt() (int, error) {
	switch n.val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(n.val.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int(n.val.Uint()), nil
	}
	return 0, conversionError(n.val, "int")
}

// GetString returns the scalar as a string, or an error otherwise
func (n *leafimpl) GetString() (string, error) {
	if n.val.Kind() == reflect.String {
		return n.val.String(), nil
	}
	return "", conversionError(n.val, "string")
}

func copyStruct(target reflect.Value, source node) error {
	targetType := target.Type()
	for i := 0; i < targetType.NumField(); i++ {
		f := targetType.Field(i)
		ch, _ := utf8.DecodeRuneInString(f.Name)
		if unicode.IsLower(ch) {
			continue
		}
		key := f.Name
		if replace := f.Tag.Get("yaml"); replace != "" {
			key = replace
		} else if replace := f.Tag.Get("json"); replace != "" {
			key = replace
		} else if replace := f.Tag.Get("mapstructure"); replace != "" {
			key = replace
		}
		child := source.GetChild(key)
		if child == nil {
			continue
		}
		err := copyAny(target.FieldByName(f.Name), child)
		if err != nil {
			return err
		}
	}
	return nil
}

func copyMap(target reflect.Value, source node) error {
	// TODO: Should handle maps with more complex types
	ktype := reflect.TypeOf("")
	vtype := reflect.TypeOf("")
	mtype := reflect.MapOf(ktype, vtype)
	results := reflect.MakeMap(mtype)

	mapKeys := source.ChildrenKeys()
	for _, mkey := range mapKeys {
		child := source.GetChild(mkey)
		if child == nil {
			continue
		}
		scalar := child.AsScalar()
		if scalar != nil {
			mval, _ := scalar.GetString()
			results.SetMapIndex(reflect.ValueOf(mkey), reflect.ValueOf(mval))
		}
	}
	target.Set(results)
	return nil
}

func copyScalar(target reflect.Value, source leaf) error {
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

func copyList(target reflect.Value, source nodeList) error {
	if source == nil {
		return fmt.Errorf("source value is not a list")
	}
	elemType := target.Type()
	elemType = elemType.Elem()
	results := reflect.MakeSlice(reflect.SliceOf(elemType), source.Size(), source.Size())
	for k := 0; k < source.Size(); k++ {
		elemSource := source.Index(k)
		ptrOut := reflect.New(elemType)
		outTarget := ptrOut.Elem()
		err := copyAny(outTarget, elemSource)
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
	if isScalar(target) {
		return copyScalar(target, source.AsScalar())
	} else if target.Kind() == reflect.Map {
		return copyMap(target, source)
	} else if target.Kind() == reflect.Struct {
		return copyStruct(target, source)
	} else if target.Kind() == reflect.Slice {
		return copyList(target, source.AsList())
	} else if target.Kind() == reflect.Invalid {
		return fmt.Errorf("can't copy invalid value %s : %v", target, target.Kind())
	}
	return fmt.Errorf("unknown value to copy: %v", target.Type())
}

func isScalar(v reflect.Value) bool {
	k := v.Kind()
	return (k >= reflect.Bool && k <= reflect.Float64) || k == reflect.String
}

func findFieldMatch(val reflect.Value, key string) int {
	schema := val.Type()
	for i := 0; i < schema.NumField(); i++ {
		f := schema.Field(i)
		name := f.Name
		if replace := f.Tag.Get("yaml"); replace != "" {
			name = replace
		} else if replace := f.Tag.Get("json"); replace != "" {
			name = replace
		} else if replace := f.Tag.Get("mapstructure"); replace != "" {
			name = replace
		}
		if name == key {
			return i
		}
	}
	return -1
}

func conversionError(v reflect.Value, expectType string) error {
	return fmt.Errorf("could not convert to %s: %v of type %T and Kind %v", expectType, v, v, v.Kind())
}
