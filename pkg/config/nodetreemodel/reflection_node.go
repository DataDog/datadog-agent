// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/config/model"
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
			item := rv.Index(i).Interface()
			elems = append(elems, item)
		}
		return newLeafNode(elems, model.SourceUnknown), nil
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
		return newLeafNode(res, model.SourceUnknown), nil
	}
	return nil, errUnknownConversion
}
