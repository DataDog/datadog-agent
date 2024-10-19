// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	// error when a caller tries to construct a node from reflect.Value, this is a logic error, calling code should
	// not be reflection based, but should be working with "native" go types that come from parsing json, yaml, etc
	errReflectValue      = fmt.Errorf("refusing to construct node from reflect.Value")
	errUnknownConversion = fmt.Errorf("no conversion found")
)

// asReflectionNode returns a node using reflection: should only show up in test code
// The reason is that data produced by parsing json, yaml, etc should always made up
// of "plain" go-lang types (maps, slices, scalars). Some unit tests assign structs directly
// to the state of the config, which then require reflection to properly handle
func asReflectionNode(v interface{}) (Node, error) {
	if _, ok := v.(reflect.Value); ok {
		return nil, errReflectValue
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Struct {
		return &structNodeImpl{val: rv}, nil
	} else if rv.Kind() == reflect.Slice {
		elems := make([]interface{}, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			node, err := NewNode(rv.Index(i).Interface())
			if err != nil {
				return nil, err
			}
			elems = append(elems, node)
		}
		return newArrayNodeImpl(elems)
	} else if rv.Kind() == reflect.Map {
		res := make(map[string]interface{}, rv.Len())
		mapkeys := rv.MapKeys()
		for _, mk := range mapkeys {
			kstr := ""
			if key, ok := mk.Interface().(string); ok {
				kstr = key
			} else {
				kstr = fmt.Sprintf("%s", mk.Interface())
			}
			res[kstr] = rv.MapIndex(mk).Interface()
		}
		return newMapNodeImpl(res)
	}
	return nil, errUnknownConversion
}

type structNodeImpl struct {
	// val must be a struct
	val reflect.Value
}

// GetChild returns the child node at the given case-insensitive key, or an error if not found
func (n *structNodeImpl) GetChild(key string) (Node, error) {
	findex := findFieldMatch(n.val, key)
	if findex == -1 {
		return nil, ErrNotFound
	}
	inner := n.val.Field(findex)
	if inner.Kind() == reflect.Interface {
		inner = inner.Elem()
	}
	return NewNode(inner.Interface())
}

// ChildrenKeys returns the list of keys of the children of the given node, if it is a map
func (n *structNodeImpl) ChildrenKeys() ([]string, error) {
	structType := n.val.Type()
	keys := make([]string, 0, n.val.NumField())
	for i := 0; i < structType.NumField(); i++ {
		f := structType.Field(i)
		ch, _ := utf8.DecodeRuneInString(f.Name)
		if unicode.IsLower(ch) {
			continue
		}
		fieldKey, _ := fieldNameToKey(f)
		keys = append(keys, fieldKey)
	}
	return keys, nil
}

type specifierSet map[string]struct{}

// fieldNameToKey returns the lower-cased field name, for case insensitive comparisons,
// with struct tag rename applied, as well as the set of specifiers from struct tags
// struct tags are handled in order of yaml, then json, then mapstructure
func fieldNameToKey(field reflect.StructField) (string, specifierSet) {
	name := field.Name

	tagtext := ""
	if val := field.Tag.Get("yaml"); val != "" {
		tagtext = val
	} else if val := field.Tag.Get("json"); val != "" {
		tagtext = val
	} else if val := field.Tag.Get("mapstructure"); val != "" {
		tagtext = val
	}

	// skip any additional specifiers such as ",omitempty" or ",squash"
	// TODO: support multiple specifiers
	var specifiers map[string]struct{}
	if commaPos := strings.IndexRune(tagtext, ','); commaPos != -1 {
		specifiers = make(map[string]struct{})
		val := tagtext[:commaPos]
		specifiers[tagtext[commaPos+1:]] = struct{}{}
		if val != "" {
			name = val
		}
	} else if tagtext != "" {
		name = tagtext
	}
	return strings.ToLower(name), specifiers
}

func findFieldMatch(val reflect.Value, key string) int {
	// case-insensitive match for struct names
	key = strings.ToLower(key)
	schema := val.Type()
	for i := 0; i < schema.NumField(); i++ {
		fieldKey, _ := fieldNameToKey(schema.Field(i))
		if key == fieldKey {
			return i
		}
	}
	return -1
}
