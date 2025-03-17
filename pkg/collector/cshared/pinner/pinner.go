// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pinner provides helpers to pin arbitrary values
package pinner

import (
	"reflect"
	"runtime"
)

// Pin pins the value with the pinner
func Pin(pinner runtime.Pinner, value any) {
	typ := reflect.TypeOf(value)
	if typ.Kind() == reflect.Ptr {
		pinner.Pin(value)
	} else if typ.Kind() == reflect.Struct {
		for i := range typ.NumField() {
			field := typ.Field(i)
			if field.Type.Kind() == reflect.Ptr {
				Pin(pinner, reflect.ValueOf(value).Field(i).Interface())
			}
		}
	}
}
