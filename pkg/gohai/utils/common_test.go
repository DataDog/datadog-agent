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
		"my_float32": "%f",
		"my_float64": "%f"
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
	// function
	f := func() {}
	_, _, err = AsJSON(&f, false)
	require.ErrorIs(t, err, ErrArgNotStruct)
	// slice
	_, _, err = AsJSON(&[]int{1}, false)
	require.ErrorIs(t, err, ErrArgNotStruct)
	// array
	_, _, err = AsJSON(&[1]int{1}, false)
	require.ErrorIs(t, err, ErrArgNotStruct)
	// basic type (bool)
	b := true
	_, _, err = AsJSON(&b, false)
	require.ErrorIs(t, err, ErrArgNotStruct)
	// pointer
	p := &struct{}{}
	_, _, err = AsJSON(&p, false)
	require.ErrorIs(t, err, ErrArgNotStruct)
}

// TestAsJsonOmit checks that fields which should be omitted are
// - type is not Value
// - no json tag
// - not exported
// - Value is an error
// - Value type is not handled (struct, ptr, bool, ...)
func TestAsJsonOmit(t *testing.T) {
	myerr := errors.New("this is an error")
	info := &struct {
		NotValue    int `json:"not_value"`
		NoTag       Value[int]
		notExported Value[int]      `json:"not_exported"` //nolint:govet
		ValueError  Value[int]      `json:"value_error"`
		ValueStruct Value[struct{}] `json:"value_struct"`
		ValuePtr    Value[*int]     `json:"value_ptr"`
		ValueBool   Value[bool]     `json:"value_bool"`
	}{
		NotValue:    1,
		NoTag:       NewValue(2),
		notExported: NewValue(3),
		ValueError:  NewErrorValue[int](myerr),
		ValueStruct: NewValue(struct{}{}),
		ValuePtr:    NewValue[*int](nil),
		ValueBool:   NewValue(true),
	}

	_, _, err := AsJSON(info, false)
	require.ErrorIs(t, err, ErrNoFieldCollected)

	marshallable, warns, err := AsJSON(info, true)
	require.NoError(t, err)
	require.ElementsMatch(t, []error{myerr}, warns)

	marshalled, err := json.Marshal(marshallable)
	require.NoError(t, err)
	expected := `{ "value_error": "0" }`
	require.JSONEq(t, expected, string(marshalled))
}

func TestAsJsonWarns(t *testing.T) {
	errs := []error{
		errors.New("this is the first error"),
		errors.New("this is the second error"),
		errors.New("this is the third error"),
		errors.New("this is the fourth error"),
	}
	info := &struct {
		FieldOne   Value[int] `json:"field_one"`
		FieldTwo   Value[int] `json:"field_two"`
		FieldThree Value[int] `json:"field_three"`
		FieldFour  Value[int] `json:"field_four"`
		FieldFive  Value[int] `json:"field_five"`
	}{
		FieldOne:   NewErrorValue[int](errs[0]),
		FieldTwo:   NewErrorValue[int](errs[1]),
		FieldThree: NewErrorValue[int](errs[2]),
		FieldFour:  NewErrorValue[int](errs[3]),
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
	errs := []error{
		errors.New("this is the first error"),
		errors.New("this is the second error"),
		errors.New("this is the third error"),
	}
	info := &struct {
		FieldOne   Value[int] `json:"field_one"`
		FieldTwo   Value[int] `json:"field_two" suffix:""`
		FieldThree Value[int] `json:"field_three" suffix:"kb"`
		FieldFour  Value[int] `json:"field_four"`
		FieldFive  Value[int] `json:"field_five" suffix:""`
		FieldSix   Value[int] `json:"field_six" suffix:"M"`
	}{
		FieldOne:   NewValue(1),
		FieldTwo:   NewValue(2),
		FieldThree: NewValue(3),
		FieldFour:  NewErrorValue[int](errs[0]),
		FieldFive:  NewErrorValue[int](errs[1]),
		FieldSix:   NewErrorValue[int](errs[2]),
	}

	marshallable, warns, err := AsJSON(info, false)
	require.NoError(t, err)
	require.ElementsMatch(t, errs, warns)

	marshalled, err := json.Marshal(marshallable)
	require.NoError(t, err)

	expected := `{
		"field_one": "1",
		"field_two": "2",
		"field_three": "3kb"
	}`
	require.JSONEq(t, expected, string(marshalled))

	marshallable, warns, err = AsJSON(info, true)
	require.NoError(t, err)
	require.ElementsMatch(t, errs, warns)

	marshalled, err = json.Marshal(marshallable)
	require.NoError(t, err)

	expected = `{
		"field_one": "1",
		"field_two": "2",
		"field_three": "3kb",
		"field_four": "0",
		"field_five": "0",
		"field_six": "0M"
	}`
	require.JSONEq(t, expected, string(marshalled))
}

func TestGetPkgName(t *testing.T) {
	require.Equal(t, "", GetPkgName(""))

	longPkgPath := "github.com/DataDog/datadog-agent/pkg/gohai/utils"
	require.Equal(t, "utils", GetPkgName(longPkgPath))
}

func TestInitializeOmit(t *testing.T) {
	info := &struct {
		NotValue    int `json:"not_value"`
		NoTag       Value[int]
		notExported Value[int] `json:"not_exported"` //nolint:govet
	}{}

	err := Initialize(info)
	require.NoError(t, err)
	// check that Initialize did not initialize any of the fields
	require.Zero(t, info.NotValue)
	require.Zero(t, info.NoTag)
	require.Zero(t, info.notExported)
}

func TestInitialize(t *testing.T) {
	info := &struct {
		SomeInt Value[int] `json:"some_int"`
	}{}
	err := Initialize(info)
	require.NoError(t, err)

	_, err = info.SomeInt.Value()
	var targetErr *NotCollectedError
	require.ErrorAs(t, err, &targetErr)
	// the struct is not declared so its package path is empty
	require.Equal(t, "", targetErr.PkgName)
	require.Equal(t, "some_int", targetErr.ValueName)
}
