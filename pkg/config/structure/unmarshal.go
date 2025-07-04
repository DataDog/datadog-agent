// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package structure defines a helper to retrieve structured data from the config
package structure

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cast"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/nodetreemodel"
)

// features allowed for handling edge-cases
type featureSet struct {
	allowSquash        bool
	convertEmptyStrNil bool
	convertArrayToMap  bool
	errorUnused        bool
}

// UnmarshalKeyOption is an option that affects the enabled features in UnmarshalKey
type UnmarshalKeyOption func(*featureSet)

// EnableSquash allows UnmarshalKey to take advantage of `mapstructure`s `squash` feature
// a squashed field hoists its fields up a level in the marshalled representation and directly embeds them
var EnableSquash UnmarshalKeyOption = func(fs *featureSet) {
	fs.allowSquash = true
}

// ErrorUnused allows UnmarshalKey to return an error if there are unused keys in the config.
var ErrorUnused UnmarshalKeyOption = func(fs *featureSet) {
	fs.errorUnused = true
}

// ConvertEmptyStringToNil allows UnmarshalKey to implicitly convert empty strings into nil slices
var ConvertEmptyStringToNil UnmarshalKeyOption = func(fs *featureSet) {
	fs.convertEmptyStrNil = true
}

// ImplicitlyConvertArrayToMapSet allows UnmarshalKey to implicitly convert an array of []interface{} to a map[interface{}]bool
var ImplicitlyConvertArrayToMapSet UnmarshalKeyOption = func(fs *featureSet) {
	fs.convertArrayToMap = true
}

// errorUnused is a mapstructure.DecoderConfig that enables erroring on unused keys
var errorUnused = func(cfg *mapstructure.DecoderConfig) {
	cfg.ErrorUnused = true
}

// legacyConvertArrayToMap convert array to map when DD_CONF_NODETREEMODEL is disabled
var legacyConvertArrayToMap = func(c *mapstructure.DecoderConfig) {
	c.DecodeHook = func(rf reflect.Kind, rt reflect.Kind, data interface{}) (interface{}, error) {
		if rf != reflect.Slice {
			return data, nil
		}
		if rt != reflect.Map {
			return data, nil
		}
		newData := map[interface{}]bool{}
		for _, i := range data.([]interface{}) {
			newData[i] = true
		}
		return newData, nil
	}
}

// UnmarshalKey retrieves data from the config at the given key and deserializes it
// to be stored on the target struct.
//
// When DD_CONF_NODETREEMODEL is enabled we use the implementation using reflection, and do not depend upon details of
// the data model of the config. Target struct can use of struct tag of "yaml", "json", or "mapstructure" to rename fields
//
// Else the viper/legacy version is used.
func UnmarshalKey(cfg model.Reader, key string, target interface{}, opts ...UnmarshalKeyOption) error {
	nodetreemodel := os.Getenv("DD_CONF_NODETREEMODEL")
	if nodetreemodel == "enable" || nodetreemodel == "unmarshal" {
		return unmarshalKeyReflection(cfg, key, target, opts...)
	}

	fs := &featureSet{}
	for _, o := range opts {
		o(fs)
	}

	decodeHooks := []func(c *mapstructure.DecoderConfig){}
	if fs.convertArrayToMap {
		decodeHooks = append(decodeHooks, legacyConvertArrayToMap)
	}
	if fs.errorUnused {
		decodeHooks = append(decodeHooks, errorUnused)
	}

	return cfg.UnmarshalKey(key, target, decodeHooks...)
}

func unmarshalKeyReflection(cfg model.Reader, key string, target interface{}, opts ...UnmarshalKeyOption) error {
	fs := &featureSet{}
	for _, o := range opts {
		o(fs)
	}
	rawval := cfg.Get(key)

	// Don't create a reflect.Value out of nil, just return immediately
	if rawval == nil {
		return nil
	}
	input, err := nodetreemodel.NewNodeTree(rawval, cfg.GetSource(key))
	if err != nil {
		return err
	}

	outValue := reflect.ValueOf(target)
	if outValue.Kind() == reflect.Pointer {
		outValue = reflect.Indirect(outValue)
	}
	rootPath := []string{}
	switch outValue.Kind() {
	case reflect.Map:
		return copyMap(outValue, input, rootPath, fs)
	case reflect.Struct:
		return copyStruct(outValue, input, rootPath, fs)
	case reflect.Slice:
		if leaf, ok := input.(nodetreemodel.LeafNode); ok {
			thing := leaf.Get()
			if arr, ok := thing.([]interface{}); ok {
				return copyList(outValue, makeNodeArray(arr), rootPath, fs)
			}
		}
		if isEmptyString(input) {
			if fs.convertEmptyStrNil {
				return nil
			}
			return fmt.Errorf("treating empty string as a nil slice not allowed for UnmarshalKey without ConvertEmptyStrNil option")
		}
		return fmt.Errorf("can not UnmarshalKey to a slice from a non-list input: %T", input)
	default:
		return fmt.Errorf("can only UnmarshalKey to struct, map, or slice, got %v", outValue.Kind())
	}
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

func copyStruct(target reflect.Value, input nodetreemodel.Node, currPath []string, fs *featureSet) error {
	targetType := target.Type()
	usedFields := make(map[string]struct{})
	for i := 0; i < targetType.NumField(); i++ {
		f := targetType.Field(i)
		ch, _ := utf8.DecodeRuneInString(f.Name)
		if unicode.IsLower(ch) {
			continue
		}
		fieldKey, specifiers := fieldNameToKey(f)
		nextPath := append(currPath, fieldKey)
		if _, ok := specifiers["squash"]; ok {
			if !fs.allowSquash {
				return fmt.Errorf("feature 'squash' not allowed for UnmarshalKey without EnableSquash option")
			}

			err := copyAny(target.FieldByName(f.Name), input, nextPath, fs)
			if err != nil {
				return err
			}
			usedFields[fieldKey] = struct{}{}
			continue
		}

		child, err := input.GetChild(fieldKey)
		if err == nodetreemodel.ErrNotFound {
			continue
		}
		if err != nil {
			return err
		}
		err = copyAny(target.FieldByName(f.Name), child, nextPath, fs)
		if err != nil {
			return err
		}
		usedFields[fieldKey] = struct{}{}
	}

	if fs.errorUnused {
		inner, ok := input.(nodetreemodel.InnerNode)
		if !ok {
			return fmt.Errorf("input is not an inner node")
		}
		var unusedKeys []string
		for _, key := range inner.ChildrenKeys() {
			if _, used := usedFields[key]; !used {
				unusedKeys = append(unusedKeys, key)
			}
		}
		if len(unusedKeys) > 0 {
			sort.Strings(unusedKeys)
			return fmt.Errorf("found unused config keys: %v", unusedKeys)
		}
	}
	return nil
}

func copyMap(target reflect.Value, input nodetreemodel.Node, currPath []string, fs *featureSet) error {
	ktype := target.Type().Key()
	vtype := target.Type().Elem()
	mtype := reflect.MapOf(ktype, vtype)
	results := reflect.MakeMap(mtype)

	if leaf, ok := input.(nodetreemodel.LeafNode); ok {
		leafValue := leaf.Get()
		if nodetreemodel.IsNilValue(leafValue) {
			return nil
		}
		if fs.convertArrayToMap {
			if arr, ok := leafValue.([]interface{}); ok {
				// convert []interface{} to map[interface{}]bool
				create := make(map[interface{}]bool)
				for k := range len(arr) {
					item := arr[k]
					create[fmt.Sprintf("%s", item)] = true
				}
				converted, err := nodetreemodel.NewNodeTree(create, model.SourceUnknown)
				if err != nil {
					return err
				}
				input = converted
			}
		}
		if m, ok := leafValue.(map[string]interface{}); ok {
			var err error
			input, err = nodetreemodel.NewNodeTree(m, model.SourceUnknown)
			if err != nil {
				return err
			}
		}
	}

	inner, ok := input.(nodetreemodel.InnerNode)
	if !ok {
		return fmt.Errorf("at %v: cannot assign to a map from input: %v of %T", currPath, input, input)
	}

	mapKeys := inner.ChildrenKeys()
	for _, mkey := range mapKeys {
		child, err := inner.GetChild(mkey)
		if err != nil {
			return err
		}
		if child == nil {
			continue
		}
		if scalar, ok := child.(nodetreemodel.LeafNode); ok {
			if mval, err := cast.ToStringE(scalar.Get()); vtype == reflect.TypeOf("") && err == nil {
				results.SetMapIndex(reflect.ValueOf(mkey), reflect.ValueOf(mval))
			} else if bval, err := cast.ToBoolE(scalar.Get()); vtype == reflect.TypeOf(true) && err == nil {
				results.SetMapIndex(reflect.ValueOf(mkey), reflect.ValueOf(bval))
			} else {
				elem := reflect.New(vtype).Elem()
				nextPath := append(currPath, mkey)
				err := copyAny(elem, child, nextPath, fs)
				if err != nil {
					return err
				}
				results.SetMapIndex(reflect.ValueOf(mkey), elem)
			}
		}
	}
	target.Set(results)
	return nil
}

func copyLeaf(target reflect.Value, input nodetreemodel.LeafNode, _ *featureSet) error {
	if input == nil {
		return fmt.Errorf("input value is not a scalar")
	}
	switch target.Kind() {
	case reflect.Bool:
		v, err := cast.ToBoolE(input.Get())
		if err != nil {
			return fmt.Errorf("could not convert %#v to bool", input.Get())
		}
		target.SetBool(v)
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v, err := cast.ToIntE(input.Get())
		if err != nil {
			return err
		}
		target.SetInt(int64(v))
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v, err := cast.ToIntE(input.Get())
		if err != nil {
			return err
		}
		target.SetUint(uint64(v))
		return nil
	case reflect.Float32, reflect.Float64:
		v, err := cast.ToFloat64E(input.Get())
		if err != nil {
			return err
		}
		target.SetFloat(float64(v))
		return nil
	case reflect.String:
		v, err := cast.ToStringE(input.Get())
		if err != nil {
			return err
		}
		target.SetString(v)
		return nil
	}
	return fmt.Errorf("unsupported scalar type %v", target.Kind())
}

func copyList(target reflect.Value, inputList []nodetreemodel.Node, currPath []string, fs *featureSet) error {
	if inputList == nil {
		return fmt.Errorf("input value is not a list")
	}
	elemType := target.Type()
	elemType = elemType.Elem()
	numElems := len(inputList)
	results := reflect.MakeSlice(reflect.SliceOf(elemType), numElems, numElems)
	for k := 0; k < numElems; k++ {
		elemSource := inputList[k]
		ptrOut := reflect.New(elemType)
		outTarget := ptrOut.Elem()
		nextPath := append(currPath, fmt.Sprintf("%d", k))
		err := copyAny(outTarget, elemSource, nextPath, fs)
		if err != nil {
			return err
		}
		results.Index(k).Set(outTarget)
	}
	target.Set(results)
	return nil
}

func copyAny(target reflect.Value, input nodetreemodel.Node, currPath []string, fs *featureSet) error {
	if target.Kind() == reflect.Pointer {
		allocPtr := reflect.New(target.Type().Elem())
		target.Set(allocPtr)
		target = allocPtr.Elem()
	}
	if isScalarKind(target) {
		if leaf, ok := input.(nodetreemodel.LeafNode); ok {
			return copyLeaf(target, leaf, fs)
		}
		if inner, ok := input.(nodetreemodel.InnerNode); ok {
			// An empty inner node is treated like a nil value, nothing to copy
			if len(inner.ChildrenKeys()) == 0 {
				return nil
			}
		}
		return fmt.Errorf("at %v: scalar required, but input is not a leaf: %v of %T", currPath, input, input)
	} else if target.Kind() == reflect.Map {
		return copyMap(target, input, currPath, fs)
	} else if target.Kind() == reflect.Struct {
		return copyStruct(target, input, currPath, fs)
	} else if target.Kind() == reflect.Slice {
		if leaf, ok := input.(nodetreemodel.LeafNode); ok {
			leafValue := leaf.Get()
			if nodetreemodel.IsNilValue(leafValue) {
				return nil
			}
			if arr, ok := leafValue.([]interface{}); ok {
				return copyList(target, makeNodeArray(arr), currPath, fs)
			}
		}
		return fmt.Errorf("at %v: []T required, but input is not an array: %v of %T", currPath, input, input)
	} else if target.Kind() == reflect.Invalid {
		return fmt.Errorf("at %v: invalid value %s : %v", currPath, target, target.Kind())
	}
	return fmt.Errorf("at %v: unknown value to copy: %v of %T", currPath, input, input)
}

func makeNodeArray(vals []interface{}) []nodetreemodel.Node {
	res := make([]nodetreemodel.Node, 0, len(vals))
	for _, v := range vals {
		node, _ := nodetreemodel.NewNodeTree(v, model.SourceUnknown)
		res = append(res, node)
	}
	return res
}

func isEmptyString(input nodetreemodel.Node) bool {
	if leaf, ok := input.(nodetreemodel.LeafNode); ok {
		if str, err := cast.ToStringE(leaf.Get()); err == nil {
			return str == ""
		}
	}
	return false
}

func isScalarKind(v reflect.Value) bool {
	k := v.Kind()
	return (k >= reflect.Bool && k <= reflect.Float64) || k == reflect.String
}
