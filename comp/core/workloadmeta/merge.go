// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"reflect"
	"time"
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
	panic("not called")
}

func timeMerge(dst, src reflect.Value) error {
	panic("not called")
}

func portSliceMerge(dst, src reflect.Value) error {
	panic("not called")
}

func mergeContainerPort(mergeMap map[string]ContainerPort, port ContainerPort) {
	panic("not called")
}

func merge(dst, src interface{}) error {
	panic("not called")
}
