// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/imdario/mergo"
)

type (
	merger struct{}
)

var (
	timeType       = reflect.TypeOf(time.Time{})
	portSliceType  = reflect.TypeOf([]ContainerPort{})
	mergerInstance = merger{}
)

func (merger) Transformer(typ reflect.Type) func(dst, src reflect.Value) error {
	switch typ {
	case timeType:
		return timeMerge
	case portSliceType:
		return portSliceMerge
	}

	return nil
}

func timeMerge(dst, src reflect.Value) error {
	if !dst.CanSet() {
		return nil
	}

	isZero := src.MethodByName("IsZero")
	result := isZero.Call([]reflect.Value{})
	if !result[0].Bool() {
		dst.Set(src)
	}
	return nil
}

func portSliceMerge(dst, src reflect.Value) error {
	if !dst.CanSet() {
		return nil
	}

	srcSlice := src.Interface().([]ContainerPort)
	dstSlice := dst.Interface().([]ContainerPort)

	// Not allocation the map if nothing to do
	if len(srcSlice) == 0 && len(dstSlice) == 0 {
		return nil
	} else if len(dstSlice) == 0 {
		dst.Set(reflect.ValueOf(srcSlice))
		return nil
	}

	mergeMap := make(map[string]ContainerPort, len(srcSlice)+len(dstSlice))
	for _, port := range dstSlice {
		mergeContainerPort(mergeMap, port)
	}
	for _, port := range srcSlice {
		mergeContainerPort(mergeMap, port)
	}

	dstSlice = make([]ContainerPort, 0, len(mergeMap))
	for id := range mergeMap {
		dstSlice = append(dstSlice, mergeMap[id])
	}
	dst.Set(reflect.ValueOf(dstSlice))

	return nil
}

// selectString will always return the first non-empty string in the input slice
// after sorting it. which will allow to do bijectif merge.
func selectString(input []string) string {
	sort.Strings(input)
	for id, s := range input {
		if s != "" {
			return input[id]
		}
	}
	return ""
}

func mergeContainerPort(mergeMap map[string]ContainerPort, inputPort ContainerPort) {
	protocol := strings.ToLower(inputPort.Protocol)
	if protocol == "tcp" {
		// consider the protocol tcp as the default one.
		protocol = ""
	}
	portKey := strconv.Itoa(inputPort.Port) + protocol
	if existingPort, found := mergeMap[portKey]; found {
		existingPort.Name = selectString([]string{existingPort.Name, inputPort.Name})
		existingPort.Protocol = selectString([]string{existingPort.Protocol, inputPort.Protocol})
		mergeMap[portKey] = existingPort
	} else {
		mergeMap[portKey] = inputPort
	}
}

func merge(dst, src interface{}) error {
	return mergo.Merge(dst, src, mergo.WithAppendSlice, mergo.WithTransformers(mergerInstance))
}
