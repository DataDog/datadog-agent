// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package structure defines a helper to retrieve structured data from the config
package structure

import (
	"encoding/json"
	"reflect"

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
