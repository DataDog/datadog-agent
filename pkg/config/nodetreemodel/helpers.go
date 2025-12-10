// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode"

	"github.com/DataDog/datadog-agent/pkg/config/helper"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/spf13/cast"
)

func splitKey(key string) []string {
	return strings.Split(strings.ToLower(key), ".")
}

func joinKey(parts ...string) string {
	nonEmptyParts := make([]string, 0, len(parts))
	for idx := range parts {
		if parts[idx] == "" {
			continue
		}
		nonEmptyParts = append(nonEmptyParts, parts[idx])
	}
	return strings.Join(nonEmptyParts, ".")
}

func safeMul(a, b uint) uint {
	c := a * b
	// detect multiplication overflows
	if a > 1 && b > 1 && c/b != a {
		return 0
	}
	return c
}

// parseSizeInBytes converts strings like 1GB or 12 mb into an unsigned integer number of bytes.
func parseSizeInBytes(sizeStr string) uint {
	sizeStr = strings.TrimSpace(sizeStr)
	lastChar := len(sizeStr) - 1
	multiplier := uint(1)

	if lastChar > 0 {
		if sizeStr[lastChar] == 'b' || sizeStr[lastChar] == 'B' {
			if lastChar > 1 {
				switch unicode.ToLower(rune(sizeStr[lastChar-1])) {
				case 'k':
					multiplier = 1 << 10
					sizeStr = strings.TrimSpace(sizeStr[:lastChar-1])
				case 'm':
					multiplier = 1 << 20
					sizeStr = strings.TrimSpace(sizeStr[:lastChar-1])
				case 'g':
					multiplier = 1 << 30
					sizeStr = strings.TrimSpace(sizeStr[:lastChar-1])
				default:
					multiplier = 1
					sizeStr = strings.TrimSpace(sizeStr[:lastChar])
				}
			}
		}
	}

	size := max(cast.ToInt(sizeStr), 0)

	return safeMul(uint(size), multiplier)
}

// ToMapStringInterface converts any type of map into a map[string]interface{}
func ToMapStringInterface(data any, path string) (map[string]interface{}, error) {
	if res, ok := data.(map[string]interface{}); ok {
		return res, nil
	}

	v := reflect.ValueOf(data)
	switch v.Kind() {
	case reflect.Map:
		convert := map[string]interface{}{}
		iter := v.MapRange()
		for iter.Next() {
			key := iter.Key()
			switch k := key.Interface().(type) {
			case string:
				convert[k] = iter.Value().Interface()
			default:
				convert[fmt.Sprintf("%v", key.Interface())] = iter.Value().Interface()
			}
		}
		return convert, nil
	}
	return nil, fmt.Errorf("expected map at '%s' got: %v", path, v)
}

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

// newNodeTree will recursively create nodes from the input value to construct a tree
func newNodeTree(v interface{}, source model.Source) (*nodeImpl, error) {
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
		node, err := newNodeTree(v, source)
		if err != nil {
			return nil, err
		}
		children[k] = node
	}
	return children, nil
}

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

func isSlice(v interface{}) bool {
	rval := reflect.ValueOf(v)
	return rval.Kind() == reflect.Slice
}
