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

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

type structNodeImpl struct {
	// val must be a struct
	val reflect.Value
}

var _ Node = (*structNodeImpl)(nil)

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
	return NewNode(inner.Interface(), model.SourceDefault)
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

// Set is not implemented for a leaf node
func (n *structNodeImpl) Set([]string, interface{}, model.Source) (bool, error) {
	return false, fmt.Errorf("not implemented")
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
