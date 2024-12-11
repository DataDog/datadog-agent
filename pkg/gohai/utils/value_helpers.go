// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package utils

import "strconv"

// ValueParseSetter returns a function which parses its input and stores the result in the given Value
func ValueParseSetter[T any](value *Value[T], parse func(string) (T, error)) func(string, error) {
	return func(val string, err error) {
		if err != nil {
			(*value) = NewErrorValue[T](err)
		} else {
			(*value) = NewValueFrom(parse(val))
		}
	}
}

// ValueStringSetter returns a function which stores a string and an error in the given Value
func ValueStringSetter(value *Value[string]) func(string, error) {
	parseFun := func(val string) (string, error) { return val, nil }
	return ValueParseSetter(value, parseFun)
}

// ValueParseInt64Setter returns a function which parses an uint64 from its string argument and
// stores the result in the given Value
func ValueParseInt64Setter(value *Value[uint64]) func(string, error) {
	parseFun := func(val string) (uint64, error) {
		return strconv.ParseUint(val, 10, 64)
	}
	return ValueParseSetter(value, parseFun)
}

// ValueParseFloat64Setter returns a function which parses a float64 from its string argument and
// stores the result in the given Value
func ValueParseFloat64Setter(value *Value[float64]) func(string, error) {
	parseFun := func(val string) (float64, error) {
		return strconv.ParseFloat(val, 64)
	}
	return ValueParseSetter(value, parseFun)
}
