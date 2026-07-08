// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package structure defines a helper to retrieve structured data from the config
package structure

import (
	"encoding/json"
	"reflect"
	"strings"

	mapstructure "github.com/go-viper/mapstructure/v2"

	"github.com/DataDog/datadog-agent/pkg/config/helper"
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

// stringToNumberSliceHookFunc is a mapstructure decode hook that parses a single
// string into a numeric slice, e.g. "53 5353" or "[53,5353]" -> []int{53,5353}
func stringToNumberSliceHookFunc() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		if f.Kind() != reflect.String || t.Kind() != reflect.Slice {
			return data, nil
		}
		switch t.Elem().Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64:
		default:
			return data, nil
		}

		s := strings.TrimSpace(data.(string))
		// A value that looks like a JSON array is decoded as JSON
		if strings.HasPrefix(s, "[") {
			dec := json.NewDecoder(strings.NewReader(s))
			dec.UseNumber() // keep large ints exact; json numbers otherwise decode as float64
			var arr []interface{}
			if err := dec.Decode(&arr); err == nil {
				return arr, nil
			}
		}
		// Otherwise treat it as a whitespace-separated list ("53 5353")
		return strings.Fields(s), nil
	}
}

func convertArrayToMap(rf reflect.Kind, rt reflect.Kind, data interface{}) (interface{}, error) {
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

// UnmarshalKey retrieves data from the config at the given key and deserializes it
// to be stored on the target struct.
func UnmarshalKey(cfg model.Reader, key string, target interface{}, opts ...UnmarshalKeyOption) error {
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

	decodeHooks := []mapstructure.DecodeHookFunc{
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
		stringToNumberSliceHookFunc(),
	}
	if fs.convertArrayToMap {
		decodeHooks = append(decodeHooks, convertArrayToMap)
	}

	dc := &mapstructure.DecoderConfig{
		Metadata:         nil,
		Result:           target,
		WeaklyTypedInput: true,
		DecodeHook:       mapstructure.ComposeDecodeHookFunc(decodeHooks...),
	}
	if fs.errorUnused {
		dc.ErrorUnused = true
	}

	var input interface{}
	if _, ok := cfg.(nodetreemodel.NodeTreeConfig); ok {
		input = cfg.Get(key)
	} else {
		input = helper.GetViperCombine(cfg, key)
	}

	decoder, err := mapstructure.NewDecoder(dc)
	if err != nil {
		return err
	}
	return decoder.Decode(input)
}
