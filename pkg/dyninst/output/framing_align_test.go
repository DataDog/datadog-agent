// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package output

import (
	"fmt"
	"reflect"
	"testing"
)

var structs = []struct {
	name string
	typ  reflect.Type
	cTyp reflect.Type
}{
	{name: "EventHeader", typ: reflect.TypeOf(EventHeader{}), cTyp: reflect.TypeOf(CEventHeader{})},
	{name: "DataItemHeader", typ: reflect.TypeOf(DataItemHeader{}), cTyp: reflect.TypeOf(CDataItemHeader{})},
}

func TestAlignment(t *testing.T) {
	for _, s := range structs {
		if s.typ.NumField() != s.cTyp.NumField() {
			t.Fatalf("header field count mismatch: %d != %d", s.typ.NumField(), s.cTyp.NumField())
		}
		for i := range s.typ.NumField() {
			field := s.typ.Field(i)
			cField := s.cTyp.Field(i)
			fmt.Println(field.Name, field.Type, field.Offset)
			fmt.Println(cField.Name, cField.Type, cField.Offset)
			if field.Offset != cField.Offset {
				t.Fatalf("field offset mismatch: %d != %d", field.Offset, cField.Offset)
			}
			if field.Type.Size() != cField.Type.Size() {
				t.Fatalf("field size mismatch: %d != %d", field.Type.Size(), cField.Type.Size())
			}
		}
		if s.typ.Size() != s.cTyp.Size() {
			t.Fatalf("header size mismatch: %d != %d", s.typ.Size(), s.cTyp.Size())
		}
	}
}
