// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

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
