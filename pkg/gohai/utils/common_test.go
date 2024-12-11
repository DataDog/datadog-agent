// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAsJSONTypes tests that each supported type can appear properly in the output
func TestAsJSONTypes(t *testing.T) {
	info := &struct {
		SomeInt     Value[int]     `json:"my_int"`
		SomeInt8    Value[int8]    `json:"my_int8"`
		SomeInt16   Value[int16]   `json:"my_int16"`
		SomeInt32   Value[int32]   `json:"my_int32"`
		SomeInt64   Value[int64]   `json:"my_int64"`
		SomeUint    Value[uint]    `json:"my_uint"`
		SomeUint8   Value[uint8]   `json:"my_uint8"`
		SomeUint16  Value[uint16]  `json:"my_uint16"`
		SomeUint32  Value[uint32]  `json:"my_uint32"`
		SomeUint64  Value[uint64]  `json:"my_uint64"`
		SomeFloat32 Value[float32] `json:"my_float32"`
		SomeFloat64 Value[float64] `json:"my_float64"`
		SomeString  Value[string]  `json:"my_string"`
	}{
		SomeInt:     NewValue(1),
		SomeInt8:    NewValue[int8](2),
		SomeInt16:   NewValue[int16](3),
		SomeInt32:   NewValue[int32](4),
		SomeInt64:   NewValue[int64](5),
		SomeUint:    NewValue[uint](6),
		SomeUint8:   NewValue[uint8](7),
		SomeUint16:  NewValue[uint16](8),
		SomeUint32:  NewValue[uint32](9),
		SomeUint64:  NewValue[uint64](10),
		SomeFloat32: NewValue[float32](32.),
		SomeFloat64: NewValue(64.),
		SomeString:  NewValue("mystr"),
	}

	marshallable, warns, err := AsJSON(info, false)
	require.NoError(t, err)
	require.Empty(t, warns)

	marshalled, err := json.Marshal(marshallable)
	require.NoError(t, err)

	// use format specifier for floats to make sure the formatting is the same
	expected := fmt.Sprintf(`{
		"my_int": "1",
		"my_int8": "2",
		"my_int16": "3",
		"my_int32": "4",
		"my_int64": "5",
		"my_uint": "6",
		"my_uint8": "7",
		"my_uint16": "8",
		"my_uint32": "9",
		"my_uint64": "10",
		"my_float32": "%v",
		"my_float64": "%v",
		"my_string": "mystr"
	}`, float32(32.), 64.)
	require.JSONEq(t, expected, string(marshalled))
}

func TestAsJSONEmpty(t *testing.T) {
	info := &struct{}{}
	_, _, err := AsJSON(info, false)
	require.ErrorIs(t, err, ErrNoFieldCollected)

	_, _, err = AsJSON(info, true)
	require.ErrorIs(t, err, ErrNoFieldCollected)
}

// TestAsJsonInvalidParam tests that using something other than struct returns an error
func TestAsJsonInvalidParam(t *testing.T) {
	var err error
	// slice
	_, _, err = AsJSON(&[]int{1}, false)
	require.ErrorIs(t, err, ErrArgNotStruct)
	// array
	_, _, err = AsJSON(&[1]int{1}, false)
	require.ErrorIs(t, err, ErrArgNotStruct)
	// pointer
	p := &struct{}{}
	_, _, err = AsJSON(&p, false)
	require.ErrorIs(t, err, ErrArgNotStruct)
}

// TestAsJsonFieldError checks that AsJSON returns an error when there is a field which cannot be exported
// - type is not Value
// - no json tag
// - not exported
// - Value inner type cannot be rendered (struct, ptr, ...)
func TestAsJsonFieldError(t *testing.T) {
	infoNotValue := &struct {
		NotValue int `json:"not_value"`
	}{
		NotValue: 1,
	}

	_, _, err := AsJSON(infoNotValue, false)
	require.ErrorIs(t, err, ErrNoValueMethod)

	infoNoTag := &struct {
		NoTag Value[int]
	}{
		NoTag: NewValue(2),
	}
	_, _, err = AsJSON(infoNoTag, false)
	require.ErrorIs(t, err, ErrNoJSONTag)

	infoNotExported := &struct {
		// use a json tag to make sure the error is due to the field not being exported
		// but govet doesn't like that
		notExported Value[int] `json:"not_exported"` //nolint:govet
	}{
		notExported: NewValue(3),
	}
	_, _, err = AsJSON(infoNotExported, false)
	require.ErrorIs(t, err, ErrNotExported)

	infoValueStruct := &struct {
		ValueStruct Value[struct{}] `json:"value_struct"`
	}{
		ValueStruct: NewValue(struct{}{}),
	}
	_, _, err = AsJSON(infoValueStruct, false)
	require.ErrorIs(t, err, ErrCannotRender)

	infoValuePtr := &struct {
		ValuePtr Value[*int] `json:"value_ptr"`
	}{
		ValuePtr: NewValue[*int](nil),
	}
	_, _, err = AsJSON(infoValuePtr, false)
	require.ErrorIs(t, err, ErrCannotRender)
}

func TestAsJsonWarns(t *testing.T) {
	errs := []string{
		"this is the first error",
		"this is the second error",
		"this is the third error",
		"this is the fourth error",
	}
	info := &struct {
		FieldOne   Value[int] `json:"field_one"`
		FieldTwo   Value[int] `json:"field_two"`
		FieldThree Value[int] `json:"field_three"`
		FieldFour  Value[int] `json:"field_four"`
		FieldFive  Value[int] `json:"field_five"`
	}{
		FieldOne:   NewErrorValue[int](errors.New(errs[0])),
		FieldTwo:   NewErrorValue[int](errors.New(errs[1])),
		FieldThree: NewErrorValue[int](errors.New(errs[2])),
		FieldFour:  NewErrorValue[int](errors.New(errs[3])),
		FieldFive:  NewValue(1),
	}

	marshallable, warns, err := AsJSON(info, false)
	require.NoError(t, err)
	require.ElementsMatch(t, errs, warns)

	marshalled, err := json.Marshal(marshallable)
	require.NoError(t, err)

	expected := `{
		"field_five": "1"
	}`
	require.JSONEq(t, expected, string(marshalled))

	marshallable, warns, err = AsJSON(info, true)
	require.NoError(t, err)
	require.ElementsMatch(t, errs, warns)

	marshalled, err = json.Marshal(marshallable)
	require.NoError(t, err)

	expected = `{
		"field_one": "0",
		"field_two": "0",
		"field_three": "0",
		"field_four": "0",
		"field_five": "1"
	}`
	require.JSONEq(t, expected, string(marshalled))
}

func TestAsJSONSuffix(t *testing.T) {
	errs := []string{
		"this is the first error",
		"this is the second error",
		"this is the third error",
	}
	info := &struct {
		FieldOne   Value[int] `json:"field_one"`
		FieldTwo   Value[int] `json:"field_two" unit:""`
		FieldThree Value[int] `json:"field_three" unit:"kb"`
		FieldFour  Value[int] `json:"field_four"`
		FieldFive  Value[int] `json:"field_five" unit:""`
		FieldSix   Value[int] `json:"field_six" unit:"M"`
	}{
		FieldOne:   NewValue(1),
		FieldTwo:   NewValue(2),
		FieldThree: NewValue(3),
		FieldFour:  NewErrorValue[int](errors.New(errs[0])),
		FieldFive:  NewErrorValue[int](errors.New(errs[1])),
		FieldSix:   NewErrorValue[int](errors.New(errs[2])),
	}

	marshallable, warns, err := AsJSON(info, true)
	require.NoError(t, err)
	require.ElementsMatch(t, errs, warns)

	marshalled, err := json.Marshal(marshallable)
	require.NoError(t, err)

	expected := `{
		"field_one": "1",
		"field_two": "2",
		"field_three": "3kb",
		"field_four": "0",
		"field_five": "0",
		"field_six": "0M"
	}`
	require.JSONEq(t, expected, string(marshalled))
}

func TestAsJSONSameError(t *testing.T) {
	info := &struct {
		FieldOne   Value[int] `json:"field_one"`
		FieldTwo   Value[int] `json:"field_two"`
		FieldThree Value[int] `json:"field_three"`
	}{
		FieldOne:   NewErrorValue[int](errors.New("this is an error")),
		FieldTwo:   NewErrorValue[int](errors.New("this is an error")),
		FieldThree: NewErrorValue[int](ErrNotCollectable),
	}

	_, warns, err := AsJSON(info, false)
	require.ErrorIs(t, err, ErrNoFieldCollected)
	require.ErrorContains(t, err, "this is an error")
	require.Empty(t, warns)
}

func TestAllEqual(t *testing.T) {
	require.True(t, allEqual([]string{}))
	require.True(t, allEqual([]string{"1"}))
	require.True(t, allEqual([]string{"1", "1"}))
	require.False(t, allEqual([]string{"1", "2"}))
}
