// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package main

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestCollectStructs_InternTag(t *testing.T) {
	src := `package example

type Foo struct {
	Name    string            ` + "`intern:\"true\"`" + `
	ID      string
	Labels  map[string]string ` + "`intern:\"true\"`" + `
	Tags    []string          ` + "`intern:\"true\"`" + `
}

type Bar struct {
	Value string
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	structs := collectStructs(f)

	fooFields := structs["Foo"]
	if len(fooFields) != 4 {
		t.Fatalf("expected 4 fields for Foo, got %d", len(fooFields))
	}

	tagged := 0
	for _, fi := range fooFields {
		if fi.internTag == "true" {
			tagged++
		}
	}
	if tagged != 3 {
		t.Errorf("expected 3 tagged fields, got %d", tagged)
	}

	barFields := structs["Bar"]
	if len(barFields) != 1 {
		t.Fatalf("expected 1 field for Bar, got %d", len(barFields))
	}
	if barFields[0].internTag != "" {
		t.Error("Bar.Value should not be tagged")
	}
}

func TestCollectStructs_EmbeddedPropagation(t *testing.T) {
	src := `package example

type Inner struct {
	X string ` + "`intern:\"true\"`" + `
}

type Outer struct {
	Inner
	Y string
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	structs := collectStructs(f)

	innerHasTag := false
	for _, fi := range structs["Inner"] {
		if fi.internTag == "true" {
			innerHasTag = true
		}
	}
	if !innerHasTag {
		t.Error("Inner should have a tagged field")
	}

	outerFields := structs["Outer"]
	hasEmbedded := false
	for _, fi := range outerFields {
		if fi.isEmbedded && fi.baseTypeName == "Inner" {
			hasEmbedded = true
		}
	}
	if !hasEmbedded {
		t.Error("Outer should have an embedded Inner field")
	}
}

func TestAnalyzeField_Types(t *testing.T) {
	src := `package example

type T struct {
	A string            ` + "`intern:\"true\"`" + `
	B map[string]string ` + "`intern:\"true\"`" + `
	C []string          ` + "`intern:\"true\"`" + `
	D *Foo
	E []Foo
	F Foo
}

type Foo struct {
	X string ` + "`intern:\"true\"`" + `
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	structs := collectStructs(f)
	fields := structs["T"]

	cases := map[string]struct {
		isString    bool
		isStringMap bool
		isStringSlc bool
		isPointer   bool
		isSlice     bool
		base        string
	}{
		"A": {isString: true},
		"B": {isStringMap: true},
		"C": {isStringSlc: true, isSlice: true},
		"D": {isPointer: true, base: "Foo"},
		"E": {isSlice: true, base: "Foo"},
		"F": {base: "Foo"},
	}

	for _, fi := range fields {
		tc, ok := cases[fi.name]
		if !ok {
			continue
		}
		if fi.isString != tc.isString {
			t.Errorf("%s: isString=%v, want %v", fi.name, fi.isString, tc.isString)
		}
		if fi.isStringMap != tc.isStringMap {
			t.Errorf("%s: isStringMap=%v, want %v", fi.name, fi.isStringMap, tc.isStringMap)
		}
		if fi.isStringSlc != tc.isStringSlc {
			t.Errorf("%s: isStringSlc=%v, want %v", fi.name, fi.isStringSlc, tc.isStringSlc)
		}
		if fi.isPointer != tc.isPointer {
			t.Errorf("%s: isPointer=%v, want %v", fi.name, fi.isPointer, tc.isPointer)
		}
		if fi.isSlice != tc.isSlice {
			t.Errorf("%s: isSlice=%v, want %v", fi.name, fi.isSlice, tc.isSlice)
		}
		if fi.baseTypeName != tc.base {
			t.Errorf("%s: base=%q, want %q", fi.name, fi.baseTypeName, tc.base)
		}
	}
}
