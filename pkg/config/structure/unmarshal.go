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
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/nodetreemodel"
)

// features allowed for handling edge-cases
type featureSet struct {
	allowSquash        bool
	convertEmptyStrNil bool
}

// UnmarshalKeyOption is an option that affects the enabled features in UnmarshalKey
type UnmarshalKeyOption func(*featureSet)

// EnableSquash allows UnmarshalKey to take advantage of `mapstructure`s `squash` feature
// a squashed field hoists its fields up a level in the marshalled representation and directly embeds them
var EnableSquash UnmarshalKeyOption = func(fs *featureSet) {
	fs.allowSquash = true
}

// ConvertEmptyStringToNil allows UnmarshalKey to implicitly convert empty strings into nil slices
var ConvertEmptyStringToNil UnmarshalKeyOption = func(fs *featureSet) {
	fs.convertEmptyStrNil = true
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
	if nodetreemodel == "enabled" || nodetreemodel == "unmarshal" {
		return unmarshalKeyReflection(cfg, key, target, opts...)
	}
	return cfg.UnmarshalKey(key, target)
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
	source, err := nodetreemodel.NewNode(rawval, cfg.GetSource(key))
	if err != nil {
		return err
	}
	outValue := reflect.ValueOf(target)
	if outValue.Kind() == reflect.Pointer {
		outValue = reflect.Indirect(outValue)
	}
	switch outValue.Kind() {
	case reflect.Map:
		return copyMap(outValue, source, fs)
	case reflect.Struct:
		return copyStruct(outValue, source, fs)
	case reflect.Slice:
		if arr, ok := source.(nodetreemodel.ArrayNode); ok {
			return copyList(outValue, arr, fs)
		}
		if isEmptyString(source) {
			if fs.convertEmptyStrNil {
				return nil
			}
			return fmt.Errorf("treating empty string as a nil slice not allowed for UnmarshalKey without ConvertEmptyStrNil option")
		}
		return fmt.Errorf("can not UnmarshalKey to a slice from a non-list source: %T", source)
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

func copyStruct(target reflect.Value, source nodetreemodel.Node, fs *featureSet) error {
	targetType := target.Type()
	for i := 0; i < targetType.NumField(); i++ {
		f := targetType.Field(i)
		ch, _ := utf8.DecodeRuneInString(f.Name)
		if unicode.IsLower(ch) {
			continue
		}
		fieldKey, specifiers := fieldNameToKey(f)
		if _, ok := specifiers["squash"]; ok {
			if !fs.allowSquash {
				return fmt.Errorf("feature 'squash' not allowed for UnmarshalKey without EnableSquash option")
			}
			err := copyAny(target.FieldByName(f.Name), source, fs)
			if err != nil {
				return err
			}
			continue
		}
		child, err := source.GetChild(fieldKey)
		if err == nodetreemodel.ErrNotFound {
			continue
		}
		if err != nil {
			return err
		}
		err = copyAny(target.FieldByName(f.Name), child, fs)
		if err != nil {
			return err
		}
	}
	return nil
}

func copyMap(target reflect.Value, source nodetreemodel.Node, _ *featureSet) error {
	// TODO: Should handle maps with more complex types in a future PR
	ktype := reflect.TypeOf("")
	vtype := reflect.TypeOf("")
	mtype := reflect.MapOf(ktype, vtype)
	results := reflect.MakeMap(mtype)

	inner, ok := source.(nodetreemodel.InnerNode)
	if !ok {
		return fmt.Errorf("can't copy a map into a leaf")
	}
	for _, mkey := range inner.ChildrenKeys() {
		child, err := inner.GetChild(mkey)
		if err != nil {
			return err
		}
		if child == nil {
			continue
		}
		if scalar, ok := child.(nodetreemodel.LeafNode); ok {
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

func copyLeaf(target reflect.Value, source nodetreemodel.LeafNode, _ *featureSet) error {
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

func copyList(target reflect.Value, source nodetreemodel.ArrayNode, fs *featureSet) error {
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
		err = copyAny(outTarget, elemSource, fs)
		if err != nil {
			return err
		}
		results.Index(k).Set(outTarget)
	}
	target.Set(results)
	return nil
}

func copyAny(target reflect.Value, source nodetreemodel.Node, fs *featureSet) error {
	if target.Kind() == reflect.Pointer {
		allocPtr := reflect.New(target.Type().Elem())
		target.Set(allocPtr)
		target = allocPtr.Elem()
	}
	if isScalarKind(target) {
		if leaf, ok := source.(nodetreemodel.LeafNode); ok {
			return copyLeaf(target, leaf, fs)
		}
		return fmt.Errorf("can't copy into target: scalar required, but source is not a leaf")
	} else if target.Kind() == reflect.Map {
		return copyMap(target, source, fs)
	} else if target.Kind() == reflect.Struct {
		return copyStruct(target, source, fs)
	} else if target.Kind() == reflect.Slice {
		if arr, ok := source.(nodetreemodel.ArrayNode); ok {
			return copyList(target, arr, fs)
		}
		return fmt.Errorf("can't copy into target: []T required, but source is not an array")
	} else if target.Kind() == reflect.Invalid {
		return fmt.Errorf("can't copy invalid value %s : %v", target, target.Kind())
	}
	return fmt.Errorf("unknown value to copy: %v", target.Type())
}

func isEmptyString(source nodetreemodel.Node) bool {
	if leaf, ok := source.(nodetreemodel.LeafNode); ok {
		if str, err := leaf.GetString(); err == nil {
			return str == ""
		}
	}
	return false
}

func isScalarKind(v reflect.Value) bool {
	k := v.Kind()
	return (k >= reflect.Bool && k <= reflect.Float64) || k == reflect.String
}
