// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"reflect"
	"regexp"

	"github.com/alecthomas/participle/lexer"
)

func inArray(value interface{}, array []interface{}) bool {
	return arrayOp(value, array, true)
}

func notInArray(value interface{}, array []interface{}) bool {
	return arrayOp(value, array, false)
}

func arrayOp(value interface{}, array []interface{}, in bool) bool {
	for _, rhs := range array {
		rhs = coerceIntegers(rhs)
		if reflect.DeepEqual(value, rhs) {
			return in
		}
	}
	return !in
}

func stringCompare(op string, lhs, rhs string, pos lexer.Position) (bool, error) {
	switch op {
	case "==":
		return lhs == rhs, nil
	case "!=":
		return lhs != rhs, nil
	case "<":
		return lhs < rhs, nil
	case ">":
		return lhs > rhs, nil
	case "<=":
		return lhs <= rhs, nil
	case ">=":
		return lhs >= rhs, nil
	case "=~", "!~":
		re, err := regexp.Compile(rhs)
		if err != nil {
			return false, lexer.Errorf(pos, `failed to parse regexp "%s" for string match using %s`, rhs, op)
		}
		match := re.MatchString(lhs)
		if op == "=~" {
			return match, nil
		}
		return !match, nil
	default:
		return false, lexer.Errorf(pos, "unsupported operator %s for string comparison", op)
	}
}

func uintCompare(op string, lhs, rhs uint64, pos lexer.Position) (bool, error) {
	switch op {
	case "==":
		return lhs == rhs, nil
	case "!=":
		return lhs != rhs, nil
	case "<":
		return lhs < rhs, nil
	case ">":
		return lhs > rhs, nil
	case "<=":
		return lhs <= rhs, nil
	case ">=":
		return lhs >= rhs, nil
	default:
		return false, lexer.Errorf(pos, "unsupported operator %s for integer comparison", op)
	}
}

func intCompare(op string, lhs, rhs int64, pos lexer.Position) (bool, error) {
	switch op {
	case "==":
		return lhs == rhs, nil
	case "!=":
		return lhs != rhs, nil
	case "<":
		return lhs < rhs, nil
	case ">":
		return lhs > rhs, nil
	case "<=":
		return lhs <= rhs, nil
	case ">=":
		return lhs >= rhs, nil
	default:
		return false, lexer.Errorf(pos, "unsupported operator %s for integer comparison", op)
	}
}

func uintBinaryOp(op string, lhs, rhs uint64, pos lexer.Position) (uint64, error) {
	switch op {
	case "&":
		return lhs & rhs, nil
	case "|":
		return lhs | rhs, nil
	case "^":
		return lhs ^ rhs, nil
	default:
		return 0, lexer.Errorf(pos, "unsupported integer binary operator %s", op)
	}
}

func intBinaryOp(op string, lhs, rhs int64, pos lexer.Position) (int64, error) {
	switch op {
	case "&":
		return lhs & rhs, nil
	case "|":
		return lhs | rhs, nil
	case "^":
		return lhs ^ rhs, nil
	default:
		return 0, lexer.Errorf(pos, "unsupported integer binary operator %s", op)
	}
}

func stringBinaryOp(op string, lhs, rhs string, pos lexer.Position) (string, error) {
	switch op {
	case "+":
		return lhs + rhs, nil
	default:
		return "", lexer.Errorf(pos, "unsupported string binary operator %s", op)
	}
}

type coerceFunc func(value interface{}) interface{}

func coerceIntegers(value interface{}) interface{} {
	switch value := value.(type) {
	case int:
		return int64(value)
	case int16:
		return int64(value)
	case int32:
		return int64(value)
	case uint:
		return uint64(value)
	case uint16:
		return uint64(value)
	case uint32:
		return uint64(value)
	}
	return value
}

func coerceArrays(value interface{}) interface{} {
	var to []interface{}

	switch from := value.(type) {
	case []int:
		for _, v := range from {
			to = append(to, int64(v))
		}
	case []int16:
		for _, v := range from {
			to = append(to, int64(v))
		}
	case []int32:
		for _, v := range from {
			to = append(to, int64(v))
		}
	case []int64:
		for _, v := range from {
			to = append(to, v)
		}
	case []uint:
		for _, v := range from {
			to = append(to, uint64(v))
		}
	case []uint16:
		for _, v := range from {
			to = append(to, uint64(v))
		}
	case []uint32:
		for _, v := range from {
			to = append(to, uint64(v))
		}
	case []uint64:
		for _, v := range from {
			to = append(to, v)
		}
	case []string:
		for _, v := range from {
			to = append(to, v)
		}
	default:
		return value
	}
	return to
}

func coerceValues(value interface{}) interface{} {
	coerceFns := []coerceFunc{
		coerceIntegers,
		coerceArrays,
	}

	for _, fn := range coerceFns {
		value = fn(value)
	}
	return value
}
