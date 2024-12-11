// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// Package utils defines helper functions to interact with protobuf definitions
package utils

import (
	"fmt"
	"reflect"
)

// ProtoCopier returns a function that will shallow copy values of a given protobuf value's type, utilising any `Get`
// prefixed method, accepting no input parameters, and returning a single value of the same type, available for each
// given field, intended to be be used with generated code for protobuf messages
// NOTE a panic will occur if the v's type is not t
func ProtoCopier(v interface{}) func(v interface{}) interface{} {
	var (
		t            = reflect.TypeOf(v)
		fieldMethods = make([][2]int, 0)
	)
	for i := 0; i < t.Elem().NumField(); i++ {
		field := t.Elem().Field(i)
		if field.PkgPath != `` {
			continue
		}
		method, ok := t.MethodByName(`Get` + field.Name)
		if !ok ||
			method.Type.NumIn() != 1 ||
			method.Type.NumOut() != 1 ||
			method.Type.Out(0) != field.Type {
			continue
		}
		fieldMethods = append(fieldMethods, [2]int{i, method.Index})
	}
	return func(v interface{}) interface{} {
		src := reflect.ValueOf(v)
		protoCopierCheckType(t, src.Type())
		dst := reflect.New(t.Elem()).Elem()
		for _, fieldMethod := range fieldMethods {
			dst.Field(fieldMethod[0]).Set(src.Method(fieldMethod[1]).Call(nil)[0])
		}
		return dst.Addr().Interface()
	}
}

func protoCopierCheckType(dst, src reflect.Type) {
	if dst != src {
		panic(fmt.Errorf(`ProtoCopier dst %s != src %s`, dst, src))
	}
}
