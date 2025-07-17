// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irprinter

import (
	"bytes"
	"fmt"
	"reflect"
	"slices"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// PrintJSON marshals the IR program to JSON.
func PrintJSON(p *ir.Program) ([]byte, error) {
	buf := bytes.Buffer{}
	enc := jsontext.NewEncoder(&buf, jsontext.ReorderRawObjects(false))

	variablesToSubprograms := make(map[*ir.Variable]*ir.Subprogram)
	for _, subprogram := range p.Subprograms {
		for _, variable := range subprogram.Variables {
			variablesToSubprograms[variable] = subprogram
		}
	}
	marshalVariable := func(enc *jsontext.Encoder, v *ir.Variable) error {
		subprogram, ok := variablesToSubprograms[v]
		if !ok {
			return fmt.Errorf("variable %s not found in any subprogram", v.Name)
		}
		idx := slices.Index(subprogram.Variables, v)
		if idx == -1 {
			return fmt.Errorf(
				"variable %s not found in subprogram %d", v.Name, subprogram.ID,
			)
		}
		return json.MarshalEncode(enc, struct {
			SubprogramID ir.SubprogramID `json:"subprogram"`
			Index        int             `json:"index"`
			Name         string          `json:"name"`
		}{
			SubprogramID: subprogram.ID,
			Index:        idx,
			Name:         v.Name,
		})
	}

	basicMarshalers := json.JoinMarshalers(
		json.MarshalToFunc(marshalTypeMap),
		// Everything we see that's a type but isn't underneath the
		// type map, we just marshal the ID.
		json.MarshalToFunc(marshalTypeAsID),
		json.MarshalToFunc(func(enc *jsontext.Encoder, v ir.PCRange) error {
			return enc.WriteToken(jsontext.String(
				fmt.Sprintf("0x%x..0x%x", v[0], v[1]),
			))
		}),
		json.MarshalToFunc(func(enc *jsontext.Encoder, v ir.ProbeKind) error {
			return enc.WriteToken(jsontext.String(v.String()))
		}),
		json.MarshalToFunc(func(enc *jsontext.Encoder, v uint64) error {
			if v < 1_000_000 {
				return json.SkipFunc
			}
			return enc.WriteToken(jsontext.String(fmt.Sprintf("0x%x", v)))
		}))
	probeMarshalers := json.JoinMarshalers(
		basicMarshalers,
		json.MarshalToFunc(func(enc *jsontext.Encoder, v *ir.Subprogram) error {
			return json.MarshalEncode(enc, struct {
				SubprogramID ir.SubprogramID `json:"subprogram"`
			}{
				SubprogramID: v.ID,
			})
		}),
	)

	// Flattens the ProbeDefinition into the probe json, keeping its
	// fields first.
	marshalProbe := func(enc *jsontext.Encoder, v *ir.Probe) (err error) {
		var buf bytes.Buffer
		if err := json.MarshalWrite(
			&buf, v.ProbeDefinition,
			json.WithMarshalers(probeMarshalers),
		); err != nil {
			return err
		}
		dec := jsontext.NewDecoder(&buf)
		defer func() {
			switch r := recover().(type) {
			case nil:
			case error:
				err = r
			default:
				panic(r)
			}
		}()
		readToken := func() jsontext.Token { return must(dec.ReadToken()) }
		readValue := func() jsontext.Value { return must(dec.ReadValue()) }
		writeToken := func(t jsontext.Token) { must(0, enc.WriteToken(t)) }
		writeValue := func(v jsontext.Value) { must(0, enc.WriteValue(v)) }
		encode := func(v any) {
			must(0, json.MarshalEncode(enc, v, json.WithMarshalers(probeMarshalers)))
		}

		beginT := readToken()
		writeToken(beginT)
		for dec.PeekKind() != '}' {
			writeToken(readToken())
			writeValue(readValue())
		}
		endT := readToken()
		writeToken(jsontext.String("subprogram"))
		encode(v.Subprogram)
		writeToken(jsontext.String("events"))
		encode(v.Events)
		writeToken(endT)
		return nil
	}
	underOperationMarshalers := json.JoinMarshalers(
		basicMarshalers,
		json.MarshalToFunc(marshalVariable),
	)
	topLevelMarshalers := json.JoinMarshalers(
		basicMarshalers,
		json.MarshalToFunc(makeOperationMarshaler(underOperationMarshalers)),
		json.MarshalToFunc(marshalProbe),
	)
	if err := json.MarshalEncode(
		enc, p,
		json.WithMarshalers(topLevelMarshalers),
		jsontext.ReorderRawObjects(false),
	); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// marshalTypeMap marshals the type map to include the root types and marshal
// them as an array as opposed to a map.
func marshalTypeMap(enc *jsontext.Encoder, tm map[ir.TypeID]ir.Type) error {
	ids := make([]ir.TypeID, 0, len(tm))
	for id := range tm {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	if err := enc.WriteToken(jsontext.BeginArray); err != nil {
		return err
	}

	for _, id := range ids {
		if err := enc.WriteToken(jsontext.BeginObject); err != nil {
			return err
		}
		if err := enc.WriteToken(jsontext.String(kindKey)); err != nil {
			return err
		}
		t := tm[id]
		tv := reflect.ValueOf(t).Elem()
		tt := tv.Type()
		tm, ok := typeMarshalers[tt]
		if !ok {
			return fmt.Errorf("no type marshaler for %s", tt)
		}
		if err := enc.WriteToken(jsontext.String(tt.Name())); err != nil {
			return err
		}
		for _, fieldPath := range tm.fields {
			v := tv.FieldByIndex(fieldPath)
			if !v.IsValid() || v.IsZero() {
				continue
			}
			field := tt.FieldByIndex(fieldPath)
			if err := enc.WriteToken(jsontext.String(field.Name)); err != nil {
				return err
			}
			if err := json.MarshalEncode(enc, v.Interface()); err != nil {
				return fmt.Errorf("failed to marshal field %s: %w", field.Name, err)
			}
		}
		if err := enc.WriteToken(jsontext.EndObject); err != nil {
			return err
		}
	}
	if err := enc.WriteToken(jsontext.EndArray); err != nil {
		return err
	}
	return nil
}

// typeMarshaler is used to help marshal a type to JSON using
// reflect.Value.
type typeMarshaler struct {
	typ    reflect.Type
	fields [][]int
}

func newTypeMarshaler(typ reflect.Type) *typeMarshaler {
	tm := &typeMarshaler{typ: typ}
	var addFields func(prefix []int, t reflect.Type)
	addFields = func(prefix []int, t reflect.Type) {
		for i, n := 0, t.NumField(); i < n; i++ {
			field := t.Field(i)
			// Recurse into anonymous fields.
			if field.Anonymous {
				path := append(prefix, i)
				path = path[:len(path):len(path)] // ensure appends realloc
				ft := field.Type
				if ft.Kind() == reflect.Pointer {
					ft = ft.Elem()
				}
				addFields(path, ft)
				continue
			}
			// Exclude unexported leaf fields.
			if !field.IsExported() {
				continue
			}
			path := append(prefix, i)
			tm.fields = append(tm.fields, path)
		}
	}
	addFields(nil, typ)
	return tm
}

// allTypes is the set of types that we know how to marshal.
var allTypes = []reflect.Type{
	reflect.TypeOf((*ir.ArrayType)(nil)),
	reflect.TypeOf((*ir.BaseType)(nil)),
	reflect.TypeOf((*ir.EventRootType)(nil)),
	reflect.TypeOf((*ir.GoChannelType)(nil)),
	reflect.TypeOf((*ir.GoEmptyInterfaceType)(nil)),
	reflect.TypeOf((*ir.GoHMapBucketType)(nil)),
	reflect.TypeOf((*ir.GoHMapHeaderType)(nil)),
	reflect.TypeOf((*ir.GoInterfaceType)(nil)),
	reflect.TypeOf((*ir.GoMapType)(nil)),
	reflect.TypeOf((*ir.GoSliceDataType)(nil)),
	reflect.TypeOf((*ir.GoSliceHeaderType)(nil)),
	reflect.TypeOf((*ir.GoStringDataType)(nil)),
	reflect.TypeOf((*ir.GoStringHeaderType)(nil)),
	reflect.TypeOf((*ir.GoSubroutineType)(nil)),
	reflect.TypeOf((*ir.GoSwissMapGroupsType)(nil)),
	reflect.TypeOf((*ir.GoSwissMapHeaderType)(nil)),
	reflect.TypeOf((*ir.PointerType)(nil)),
	reflect.TypeOf((*ir.StructureType)(nil)),
	reflect.TypeOf((*ir.VoidPointerType)(nil)),
}

var typeMarshalers map[reflect.Type]*typeMarshaler
var typesByName map[string]reflect.Type

func init() {
	typeMarshalers = make(map[reflect.Type]*typeMarshaler, len(allTypes))
	typesByName = make(map[string]reflect.Type, len(allTypes))
	for _, tp := range allTypes {
		t := tp.Elem()
		typeMarshalers[t] = newTypeMarshaler(t)
		typesByName[t.Name()] = t
	}
}

const kindKey = "__kind"

func marshalTypeAsID(enc *jsontext.Encoder, t ir.Type) error {
	typeName := reflect.TypeOf(t).Elem().Name()
	return enc.WriteToken(jsontext.String(fmt.Sprintf("%d %s %s", t.GetID(), typeName, t.GetName())))
}

func makeOperationMarshaler(
	marshalers *json.Marshalers,
) func(enc *jsontext.Encoder, op ir.ExpressionOp) error {
	return func(enc *jsontext.Encoder, op ir.ExpressionOp) error {
		switch op := op.(type) {
		case *ir.LocationOp:
			type locationWithKind struct {
				Kind string         `json:"__kind"`
				Op   *ir.LocationOp `json:",inline"`
			}
			return json.MarshalEncode(enc, locationWithKind{
				Kind: "LocationOp",
				Op:   op,
			}, json.WithMarshalers(marshalers))
		default:
			return fmt.Errorf("unknown operation: %T", op)
		}
	}
}
