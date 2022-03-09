// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"reflect"
	"time"

	"github.com/imdario/mergo"
)

type timeMerger struct{}

var (
	timeType           = reflect.TypeOf(time.Time{})
	timeMergerInstance = timeMerger{}
)

func (tm timeMerger) Transformer(typ reflect.Type) func(dst, src reflect.Value) error {
	if typ != timeType {
		return nil
	}

	return func(dst, src reflect.Value) error {
		if dst.CanSet() {
			isZero := src.MethodByName("IsZero")
			result := isZero.Call([]reflect.Value{})
			if !result[0].Bool() {
				dst.Set(src)
			}
		}
		return nil
	}
}

func merge(dst, src interface{}) error {
	return mergo.Merge(dst, src, mergo.WithTransformers(timeMergerInstance))
}
