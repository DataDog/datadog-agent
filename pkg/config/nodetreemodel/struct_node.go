// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"
	"reflect"
	"slices"
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
var _ InnerNode = (*structNodeImpl)(nil)

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
	return NewNodeTree(inner.Interface(), model.SourceDefault)
}

func (n *structNodeImpl) HasChild(name string) bool {
	names := n.ChildrenKeys()
	return slices.Contains(names, name)
}

func (n *structNodeImpl) Merge(InnerNode) (InnerNode, error) {
	return nil, fmt.Errorf("not implemented")
}

// ChildrenKeys returns the list of keys of the children of the given node, if it is a map
func (n *structNodeImpl) ChildrenKeys() []string {
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
	return keys
}

// SetAt is not implemented for a struct node
func (n *structNodeImpl) SetAt([]string, interface{}, model.Source) error {
	return fmt.Errorf("not implemented")
}

// InsertChildNode is not implemented for a struct node
func (n *structNodeImpl) InsertChildNode(string, Node) {}

// RemoveChild is not implemented for struct node
func (n *structNodeImpl) RemoveChild(string) {}

// Clone clones a StructNode
func (n *structNodeImpl) Clone() Node {
	return &structNodeImpl{val: n.val}
}

// SourceGreaterOrEqual returns true if the source of the current node is greater or equal to the one given as a
// parameter
func (n *structNodeImpl) SourceGreaterOrEqual(model.Source) bool {
	return false
}

func (n *structNodeImpl) Get() interface{} {
	return nil
}

// SetWithSource assigns a value in the config, for the given source
func (n *structNodeImpl) SetWithSource(interface{}, model.Source) error {
	return fmt.Errorf("not implemented")
}

// Source returns the source for this leaf
func (n *structNodeImpl) Source() model.Source {
	return model.SourceUnknown
}

// DumpSettings returns nil
func (n *structNodeImpl) DumpSettings(func(model.Source) bool) map[string]interface{} {
	return nil
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
