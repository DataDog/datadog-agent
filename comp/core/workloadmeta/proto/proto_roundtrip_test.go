// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package proto

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// enumValues maps Go types that have proto enum conversions to valid sample values.
// Without this, populateStruct would set a string like "test_Runtime" which
// doesn't map to a valid proto enum.
var enumValues = map[reflect.Type]interface{}{
	reflect.TypeOf(workloadmeta.Kind("")): workloadmeta.KindContainer,
}

// maxPopulateDepth prevents infinite recursion when populating self-referencing
// types (e.g. CycloneDX BOM components with recursive Pedigree.Ancestors).
const maxPopulateDepth = 10

// populateStruct recursively fills every field in a struct with a non-zero
// value using reflection. This ensures that when we round-trip through proto,
// any field that silently drops will show up as a zero value vs a non-zero
// original — catching missing conversion code.
//
// Fields tagged with `proto:"ignore"` are skipped during population since they
// won't be compared in the round-trip.
func populateStruct(t *testing.T, v reflect.Value, path string, depth int) {
	t.Helper()

	if depth > maxPopulateDepth {
		return
	}

	switch v.Kind() {
	case reflect.Struct:
		// Handle time.Time specially
		if v.Type() == reflect.TypeOf(time.Time{}) {
			v.Set(reflect.ValueOf(time.Unix(1700000000, 0)))
			return
		}

		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			fieldType := v.Type().Field(i)
			if !field.CanSet() {
				continue
			}
			if fieldType.Tag.Get("proto") == "ignore" {
				continue
			}
			fieldPath := path + "." + fieldType.Name
			populateStruct(t, field, fieldPath, depth+1)
		}

	case reflect.String:
		if ev, ok := enumValues[v.Type()]; ok {
			v.Set(reflect.ValueOf(ev))
		} else {
			v.SetString("test_" + path)
		}

	case reflect.Int, reflect.Int32, reflect.Int64:
		if ev, ok := enumValues[v.Type()]; ok {
			v.Set(reflect.ValueOf(ev))
		} else if v.Type() == reflect.TypeOf(time.Duration(0)) {
			v.Set(reflect.ValueOf(time.Duration(42)))
		} else {
			v.SetInt(42)
		}

	case reflect.Uint8, reflect.Uint16, reflect.Uint64:
		v.SetUint(42)

	case reflect.Float64:
		v.SetFloat(3.14)

	case reflect.Bool:
		v.SetBool(true)

	case reflect.Slice:
		if v.Type() == reflect.TypeOf([]byte(nil)) {
			v.SetBytes([]byte("test"))
			return
		}
		elem := reflect.New(v.Type().Elem()).Elem()
		populateStruct(t, elem, path+"[0]", depth+1)
		v.Set(reflect.Append(v, elem))

	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		key := reflect.New(v.Type().Key()).Elem()
		val := reflect.New(v.Type().Elem()).Elem()
		populateStruct(t, key, path+".key", depth+1)
		populateStruct(t, val, path+".val", depth+1)
		m.SetMapIndex(key, val)
		v.Set(m)

	case reflect.Ptr:
		elem := reflect.New(v.Type().Elem())
		populateStruct(t, elem.Elem(), path, depth+1)
		v.Set(elem)

	default:
		t.Logf("populateStruct: unhandled kind %s at %s", v.Kind(), path)
	}
}

// protoFieldHint is appended to every round-trip comparison failure to guide
// developers toward the correct fix.
const protoFieldHint = "\n\tEither update the proto conversion in workloadmeta/proto to include this field, " +
	"or add `proto:\"ignore\"` to the struct tag in types.go if it should not be serialized."

// compareFields recursively compares struct fields between original and result,
// reporting which specific fields were lost in the round-trip. Fields tagged
// with `proto:"ignore"` on the struct definition are skipped automatically.
func compareFields(t *testing.T, original, result reflect.Value, path string) {
	t.Helper()

	if original.Kind() == reflect.Ptr {
		if original.IsNil() {
			return
		}
		if result.IsNil() {
			t.Errorf("field %s was not round-tripped through proto (original is non-nil, result is nil).%s", path, protoFieldHint)
			return
		}
		compareFields(t, original.Elem(), result.Elem(), path)
		return
	}

	if original.Kind() != reflect.Struct {
		if !reflect.DeepEqual(original.Interface(), result.Interface()) {
			t.Errorf("field %s was not round-tripped through proto (got %v, want %v).%s", path, result.Interface(), original.Interface(), protoFieldHint)
		}
		return
	}

	// Handle time.Time: compare via Unix() since proto loses nanoseconds
	if original.Type() == reflect.TypeOf(time.Time{}) {
		origTime := original.Interface().(time.Time)
		resultTime := result.Interface().(time.Time)
		if origTime.Unix() != resultTime.Unix() {
			t.Errorf("field %s was not round-tripped through proto (got %v, want %v).%s", path, resultTime, origTime, protoFieldHint)
		}
		return
	}

	for i := 0; i < original.NumField(); i++ {
		fieldType := original.Type().Field(i)
		fieldName := fieldType.Name
		fieldPath := path + "." + fieldName

		if fieldType.Tag.Get("proto") == "ignore" {
			continue
		}

		origField := original.Field(i)
		resultField := result.Field(i)

		// For external package types (outside workloadmeta), just check non-zero.
		// This also covers slices of external types (e.g. []TracerMetadata).
		elemType := fieldType.Type
		if elemType.Kind() == reflect.Slice {
			elemType = elemType.Elem()
		}
		if elemType.Kind() == reflect.Ptr {
			elemType = elemType.Elem()
		}
		if isExternalType(elemType) {
			if !origField.IsZero() && resultField.IsZero() {
				t.Errorf("field %s was not round-tripped through proto (external type is non-zero in original but zero in result).%s", fieldPath, protoFieldHint)
			}
			continue
		}

		switch origField.Kind() {
		case reflect.Struct:
			compareFields(t, origField, resultField, fieldPath)

		case reflect.Ptr:
			if origField.IsNil() {
				continue
			}
			if resultField.IsNil() {
				t.Errorf("field %s was not round-tripped through proto (original is non-nil, result is nil).%s", fieldPath, protoFieldHint)
				continue
			}
			compareFields(t, origField.Elem(), resultField.Elem(), fieldPath)

		case reflect.Slice:
			if origField.Len() != resultField.Len() {
				t.Errorf("field %s was not round-tripped through proto (slice length: got %d, want %d).%s", fieldPath, resultField.Len(), origField.Len(), protoFieldHint)
				continue
			}
			for j := 0; j < origField.Len(); j++ {
				elemPath := fmt.Sprintf("%s[%d]", fieldPath, j)
				compareFields(t, origField.Index(j), resultField.Index(j), elemPath)
			}

		case reflect.Map:
			if origField.Len() != resultField.Len() {
				t.Errorf("field %s was not round-tripped through proto (map length: got %d, want %d).%s", fieldPath, resultField.Len(), origField.Len(), protoFieldHint)
				continue
			}
			if !reflect.DeepEqual(origField.Interface(), resultField.Interface()) {
				t.Errorf("field %s was not round-tripped through proto (map contents changed).%s", fieldPath, protoFieldHint)
			}

		default:
			if !reflect.DeepEqual(origField.Interface(), resultField.Interface()) {
				t.Errorf("field %s was not round-tripped through proto (got %v, want %v).%s", fieldPath, resultField.Interface(), origField.Interface(), protoFieldHint)
			}
		}
	}
}

// isExternalType returns true if the type's package is outside the workloadmeta
// definition package. External types are only checked for non-zero (we don't
// recurse into their fields since we don't control them).
func isExternalType(t reflect.Type) bool {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return false
	}
	pkg := t.PkgPath()
	if pkg == "" {
		return false
	}
	return !strings.Contains(pkg, "comp/core/workloadmeta/def")
}

// TestProtoRoundTripFieldCoverage uses reflection to populate all fields of a
// workloadmeta entity, round-trips it through proto serialization, and verifies
// every field survives. Fields intentionally excluded from proto must be tagged
// with `proto:"ignore"` on the struct definition — if a new field is added to
// the struct but not to proto conversion code AND not tagged, this test fails.
func TestProtoRoundTripFieldCoverage(t *testing.T) {
	tests := []struct {
		name   string
		entity workloadmeta.Entity
		setup  func(workloadmeta.Entity)
	}{
		{
			name:   "Container",
			entity: &workloadmeta.Container{},
			setup: func(e workloadmeta.Entity) {
				c := e.(*workloadmeta.Container)
				c.EntityID.Kind = workloadmeta.KindContainer
				c.Runtime = workloadmeta.ContainerRuntimeDocker
				c.State.Status = workloadmeta.ContainerStatusRunning
				c.State.Health = workloadmeta.ContainerHealthHealthy
			},
		},
		{
			name:   "KubernetesPod",
			entity: &workloadmeta.KubernetesPod{},
			setup: func(e workloadmeta.Entity) {
				e.(*workloadmeta.KubernetesPod).EntityID.Kind = workloadmeta.KindKubernetesPod
			},
		},
		{
			name:   "ECSTask",
			entity: &workloadmeta.ECSTask{},
			setup: func(e workloadmeta.Entity) {
				task := e.(*workloadmeta.ECSTask)
				task.EntityID.Kind = workloadmeta.KindECSTask
				task.LaunchType = workloadmeta.ECSLaunchTypeEC2
			},
		},
		{
			name:   "Process",
			entity: &workloadmeta.Process{},
			setup: func(e workloadmeta.Entity) {
				e.(*workloadmeta.Process).EntityID.Kind = workloadmeta.KindProcess
			},
		},
		{
			name:   "ContainerImageMetadata",
			entity: &workloadmeta.ContainerImageMetadata{},
			setup: func(e workloadmeta.Entity) {
				e.(*workloadmeta.ContainerImageMetadata).EntityID.Kind = workloadmeta.KindContainerImageMetadata
			},
		},
		{
			name:   "CRD",
			entity: &workloadmeta.CRD{},
			setup: func(e workloadmeta.Entity) {
				e.(*workloadmeta.CRD).EntityID.Kind = workloadmeta.KindCRD
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Populate all fields with non-zero values via reflection
			entityVal := reflect.ValueOf(tc.entity).Elem()
			populateStruct(t, entityVal, tc.name, 0)

			// Apply entity-specific overrides (enum values, Kind, etc.)
			tc.setup(tc.entity)

			// Round-trip: Go → proto → Go
			event := workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: tc.entity}
			protoEvent, err := ProtobufEventFromWorkloadmetaEvent(event)
			require.NoError(t, err)

			resultEvent, err := WorkloadmetaEventFromProtoEvent(protoEvent)
			require.NoError(t, err)

			// Compare every field between original and round-tripped entity
			resultVal := reflect.ValueOf(resultEvent.Entity).Elem()
			compareFields(t, entityVal, resultVal, tc.name)
		})
	}
}
