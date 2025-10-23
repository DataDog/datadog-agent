// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package structure defines a helper to retrieve structured data from the config
package structure

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	mapstructure "github.com/go-viper/mapstructure/v2"
	"github.com/spf13/cast"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/nodetreemodel"
)

// features allowed for handling edge-cases
type featureSet struct {
	allowSquash        bool
	stringUnmarshal    bool
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

// EnableStringUnmarshal allows UnmarshalKey to handle stringified json and Unmarshal it
var EnableStringUnmarshal UnmarshalKeyOption = func(fs *featureSet) {
	fs.stringUnmarshal = true
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

	if fs.stringUnmarshal {
		rawval := cfg.Get(key)
		if rawval == nil {
			return nil
		}
		if str, ok := rawval.(string); ok {
			if str == "" {
				return nil
			}
			return json.Unmarshal([]byte(str), &target)
		}
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

// buildTreeFromConfigSettings creates a map of values by merging settings from each config source
func buildTreeFromConfigSettings(cfg model.Reader, key string) (interface{}, error) {
	rawval := cfg.Get(key)
	if nodetreemodel.IsNilValue(rawval) {
		// NOTE: This returns a nil-valued-interface, which is needed to handle edge
		// cases in the same way viper does
		var ret map[string]interface{}
		return ret, nil
	}

	mapval, ok := rawval.(map[string]interface{})
	if !ok {
		return rawval, nil
	}
	tree := make(map[string]interface{})
	for k, v := range mapval {
		tree[k] = v
	}

	fields := cfg.GetSubfields(key)
	for _, f := range fields {
		setting := strings.Join([]string{key, f}, ".")
		inner, _ := buildTreeFromConfigSettings(cfg, setting)
		if inner == nil {
			continue
		}
		if nodetreemodel.IsNilValue(inner) {
			// NOTE: This returns a nil-valued-interface, which is needed to handle edge
			// cases in the same way viper does
			var ret map[string]interface{}
			inner = ret
		}
		tree[f] = inner
	}

	return tree, nil
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

	if fs.stringUnmarshal {
		if str, ok := rawval.(string); ok {
			if str == "" {
				return nil
			}
			return json.Unmarshal([]byte(str), &target)
		}
	}

	var inputNode nodetreemodel.Node
	if nodeConfig, ok := cfg.(nodetreemodel.NodeTreeConfig); ok {
		node, err := nodeConfig.GetNode(key)
		if err != nil {
			return err
		}
		inputNode = node
	} else {
		settingval, err := buildTreeFromConfigSettings(cfg, key)
		if err != nil {
			return err
		}

		node, err := nodetreemodel.NewNodeTree(settingval, cfg.GetSource(key))
		if err != nil {
			return err
		}
		inputNode = node
	}

	outValue := reflect.ValueOf(target)
	// Resolve pointers 2 times. This is needed because callers often do this:
	//
	// mystruct := &MyStruct{}
	// err := structure.UnmarshalKey(config, "my_key", &mystruct)
	//
	// It would take highly unusual code to have more indirection than this.
	if outValue.Kind() == reflect.Pointer {
		outValue = reflect.Indirect(outValue)
	}
	if outValue.Kind() == reflect.Pointer {
		outValue = reflect.Indirect(outValue)
	}
	rootPath := []string{}
	switch outValue.Kind() {
	case reflect.Map:
		return copyMap(outValue, inputNode, rootPath, fs)
	case reflect.Struct:
		return copyStruct(outValue, inputNode, rootPath, fs)
	case reflect.Slice:
		if leaf, ok := inputNode.(nodetreemodel.LeafNode); ok {
			thing := leaf.Get()
			nodeArray, err := makeNodeArray(thing)
			if err != nil {
				return fmt.Errorf("can not UnmarshalKey to slice from non-list input: %v of %T", thing, thing)
			}
			return copyList(outValue, nodeArray, rootPath, fs)
		}
		if isEmptyString(inputNode) {
			if fs.convertEmptyStrNil {
				return nil
			}
			return fmt.Errorf("treating empty string as a nil slice not allowed for UnmarshalKey without ConvertEmptyStrNil option")
		}
		return fmt.Errorf("can not UnmarshalKey to a slice from a non-list input: %T", inputNode)
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

	// extract specifier tags such as ",omitempty" or ",squash"
	specifiers := make(map[string]struct{})
	for i, val := range strings.Split(tagtext, ",") {
		if i == 0 && val != "" {
			name = val
			continue
		}
		specifiers[val] = struct{}{}
	}
	return strings.ToLower(name), specifiers
}

func copyStruct(target reflect.Value, input nodetreemodel.Node, currPath []string, fs *featureSet) error {
	if leafNode, ok := input.(nodetreemodel.LeafNode); ok {
		m, err := nodetreemodel.ToMapStringInterface(leafNode.Get(), strings.Join(currPath, "."))
		if err != nil {
			return err
		}
		converted, err := nodetreemodel.NewNodeTree(m, model.SourceUnknown)
		if err != nil {
			return err
		}
		input = converted
	}

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
		} else {
			obj, err := nodetreemodel.ToMapStringInterface(leafValue, strings.Join(currPath, "."))
			if err != nil {
				return err
			}
			input, err = nodetreemodel.NewNodeTree(obj, model.SourceUnknown)
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
		// Convert to the target key type, supports type aliases like map[ResourceType]string
		realkey := reflect.ValueOf(mkey).Convert(ktype)
		if scalar, ok := child.(nodetreemodel.LeafNode); ok {
			if mval, err := cast.ToStringE(scalar.Get()); vtype == reflect.TypeOf("") && err == nil {
				results.SetMapIndex(realkey, reflect.ValueOf(mval))
			} else if bval, err := cast.ToBoolE(scalar.Get()); vtype == reflect.TypeOf(true) && err == nil {
				results.SetMapIndex(realkey, reflect.ValueOf(bval))
			} else {
				elem := reflect.New(vtype).Elem()
				nextPath := append(currPath, mkey)
				err := copyAny(elem, child, nextPath, fs)
				if err != nil {
					return err
				}
				results.SetMapIndex(realkey, elem)
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

	// If types already match, just copy directly
	inVal := input.Get()
	if inVal != nil && target.Type() == reflect.ValueOf(inVal).Type() {
		target.Set(reflect.ValueOf(inVal))
		return nil
	}

	switch target.Kind() {
	case reflect.Bool:
		v, err := cast.ToBoolE(inVal)
		if err != nil {
			return fmt.Errorf("could not convert %#v to bool", inVal)
		}
		target.SetBool(v)
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		v, err := cast.ToIntE(inVal)
		if err != nil {
			return err
		}
		target.SetInt(int64(v))
		return nil
	case reflect.Int64:
		v, err := cast.ToInt64E(inVal)
		if err != nil {
			return err
		}
		target.SetInt(int64(v))
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		v, err := cast.ToUintE(inVal)
		if err == nil {
			target.SetUint(uint64(v))
			return nil
		}
		// If input is a negative int, cast.ToUint won't work, force a conversion
		// by wrapping around the value
		if num, converts := inVal.(int); converts {
			target.SetUint(uint64(num))
			return nil
		}
		return err
	case reflect.Uint64:
		v, err := cast.ToUint64E(inVal)
		if err == nil {
			target.SetUint(uint64(v))
			return nil
		}
		if num, converts := inVal.(int); converts {
			target.SetUint(uint64(num))
			return nil
		}
		return err
	case reflect.Float32, reflect.Float64:
		v, err := cast.ToFloat64E(inVal)
		if err != nil {
			return err
		}
		target.SetFloat(float64(v))
		return nil
	case reflect.String:
		v, err := cast.ToStringE(inVal)
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
	} else if target.Kind() == reflect.Interface {
		// If the target is an interface{}, assume it's a scalar since it's likely part of a
		// heterogeneous slice like []interface{}. Don't use copyAny since that expects to
		// understand a concrete scalar type, instead simply copy the value using reflection.
		if leaf, ok := input.(nodetreemodel.LeafNode); ok {
			target.Set(reflect.ValueOf(leaf.Get()))
			return nil
		}
		return fmt.Errorf("at %v: can't copy inner node to interface: %v of %T", currPath, input, input)
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

			nodeArray, err := makeNodeArray(leafValue)
			if err != nil {
				return fmt.Errorf("at %v, []T required, but input is not an array: %v of %T", currPath, input, input)
			}
			return copyList(target, nodeArray, currPath, fs)
		}
		return fmt.Errorf("at %v: []T required, but input is not an array: %v of %T", currPath, input, input)
	} else if target.Kind() == reflect.Invalid {
		return fmt.Errorf("at %v: invalid value %s : %v", currPath, target, target.Kind())
	}
	return fmt.Errorf("at %v: unknown value to copy: %v of %T", currPath, input, input)
}

func makeNodeArray(vals interface{}) ([]nodetreemodel.Node, error) {
	s := reflect.ValueOf(vals)
	if s.Kind() != reflect.Slice {
		return nil, fmt.Errorf("value is not a slice")
	}

	res := make([]nodetreemodel.Node, 0, s.Len())
	for i := 0; i < s.Len(); i++ {
		node, _ := nodetreemodel.NewNodeTree(s.Index(i).Interface(), model.SourceUnknown)
		res = append(res, node)
	}
	return res, nil
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
