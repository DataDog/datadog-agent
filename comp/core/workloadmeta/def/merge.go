// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"reflect"
	"sort"
	"strconv"
	"time"

	"github.com/imdario/mergo"
)

type (
	merger struct{}
)

var (
	timeType            = reflect.TypeOf(time.Time{})
	portSliceType       = reflect.TypeOf([]ContainerPort{})
	containerHealthType = reflect.TypeOf(ContainerHealthUnknown)
	mergerInstance      = merger{}
)

func (merger) Transformer(typ reflect.Type) func(dst, src reflect.Value) error {
	switch typ {
	case timeType:
		return timeMerge
	case portSliceType:
		return portSliceMerge
	// Even though Health is string alias, the matching only matches actual Health
	case containerHealthType:
		return healthMerge
	}

	return nil
}

func healthMerge(dst, src reflect.Value) error {
	if !dst.CanSet() {
		return nil
	}

	srcHealth := src.Interface().(ContainerHealth)
	dstHealth := dst.Interface().(ContainerHealth)

	if srcHealth != "" && srcHealth != ContainerHealthUnknown && (dstHealth == "" || dstHealth == ContainerHealthUnknown) {
		dst.Set(src)
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
	if len(srcSlice) == 0 {
		return nil
	}
	if len(dstSlice) == 0 {
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
	for _, port := range mergeMap {
		dstSlice = append(dstSlice, port)
	}
	sort.Slice(dstSlice, func(i, j int) bool {
		if dstSlice[i].Port < dstSlice[j].Port {
			return true
		}
		if dstSlice[i].Port > dstSlice[j].Port {
			return false
		}
		return dstSlice[i].Protocol < dstSlice[j].Protocol
	})
	dst.Set(reflect.ValueOf(dstSlice))

	return nil
}

func mergeContainerPort(mergeMap map[string]ContainerPort, port ContainerPort) {
	portKey := strconv.Itoa(port.Port) + port.Protocol
	existingPort, found := mergeMap[portKey]

	if found {
		if (existingPort.Name == "" && port.Name != "") ||
			(existingPort.HostPort == 0 && port.HostPort != 0) {
			mergeMap[portKey] = port
		}
	} else {
		mergeMap[portKey] = port
	}
}

func merge(dst, src interface{}) error {
	return mergo.Merge(dst, src, mergo.WithAppendSlice, mergo.WithTransformers(mergerInstance))
}
