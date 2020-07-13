// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package eval

import (
	"errors"
	"testing"

	"github.com/alecthomas/participle/lexer"
	assert "github.com/stretchr/testify/require"
)

type instanceTest struct {
	name         string
	expression   string
	vars         VarMap
	functions    FunctionMap
	expectResult interface{}
	expectError  error
}

func (test instanceTest) Run(t *testing.T) {
	t.Helper()
	assert := assert.New(t)
	expr, err := ParseExpression(test.expression)
	assert.NoError(err)
	assert.NotNil(expr)

	instance := &Instance{
		Functions: test.functions,
		Vars:      test.vars,
	}
	result, err := expr.Evaluate(instance)
	if test.expectError != nil {
		assert.Equal(test.expectError, err)
	} else {
		assert.NoError(err)
		assert.Equal(test.expectResult, result)
	}
}

type instanceTests []instanceTest

func (tests instanceTests) Run(t *testing.T) {
	t.Helper()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.Run(t)
		})
	}
}

func newLexerError(offset int, msg string) error {
	return lexer.Errorf(
		lexer.Position{
			Offset: offset,
			Column: offset + 1,
			Line:   1,
		},
		msg,
	)
}

func TestEvalFunction(t *testing.T) {
	instanceTests{
		{
			name:       "string function",
			expression: `ping("pong") == "pong"`,
			functions: FunctionMap{
				"ping": func(instance *Instance, args ...interface{}) (interface{}, error) {
					return args[0].(string), nil
				},
			},
			expectResult: true,
		},
		{
			name:       "unknown function",
			expression: `hey("you")`,

			expectError: newLexerError(0, `unknown function "hey()"`),
		},
		{
			name:       "function error",
			expression: `hey("you")`,
			functions: FunctionMap{
				"hey": func(instance *Instance, args ...interface{}) (interface{}, error) {
					return nil, errors.New("hey failed")
				},
			},
			expectError: newLexerError(0, `call to "hey()" failed`),
		},
		{
			name:       "function arg evaluation error",
			expression: `hey(you)`,
			functions: FunctionMap{
				"hey": func(instance *Instance, args ...interface{}) (interface{}, error) {
					return nil, nil
				},
			},
			expectError: newLexerError(4, `unknown variable "you"`),
		},
	}.Run(t)
}

func TestEvalBoolean(t *testing.T) {
	instanceTests{
		{
			name:       "not",
			expression: `!x`,
			vars: VarMap{
				"x": true,
			},
			expectResult: false,
		},
		{
			name:       "and",
			expression: `x && y`,
			vars: VarMap{
				"x": true,
				"y": false,
			},
			expectResult: false,
		},
		{
			name:       "or",
			expression: `x || y`,
			vars: VarMap{
				"x": true,
				"y": false,
			},
			expectResult: true,
		},
		{
			name:       "invalid not",
			expression: `!x`,
			vars: VarMap{
				"x": "abc",
			},
			expectError: newLexerError(0, "rhs of ! must be a boolean"),
		},
		{
			name:        "invalid and",
			expression:  `"x" && "y"`,
			expectError: newLexerError(0, "type mismatch, expected bool in lhs of boolean expression"),
		},
		{
			name:        "invalid or",
			expression:  `"x" || "y"`,
			expectError: newLexerError(0, "type mismatch, expected bool in lhs of boolean expression"),
		},
	}.Run(t)
}

func TestEvalInteger(t *testing.T) {
	instanceTests{
		{
			name:         "octal",
			expression:   "0644",
			expectResult: uint64(0644),
		},
		{
			name:         "hex",
			expression:   "0xff",
			expectResult: uint64(0xff),
		},
		{
			name:         "unsigned equal 0",
			expression:   `0xff == 0`,
			expectResult: false,
		},
		{
			name:         "unsigned not equal 0",
			expression:   `0xff != 0`,
			expectResult: true,
		},
		{
			name:         "unsigned less than 0",
			expression:   `0xff < 0`,
			expectResult: false,
		},
		{
			name:         "unsigned less than or equal 0",
			expression:   `0x0 <= 0`,
			expectResult: true,
		},
		{
			name:         "unsigned greater than 0",
			expression:   `0xff > 0`,
			expectResult: true,
		},
		{
			name:         "unsigned greater than or equal 0",
			expression:   `0x0 >= 0`,
			expectResult: true,
		},
		{
			name:       "unsigned greater than signed",
			expression: `0x9 > x`,
			vars: VarMap{
				"x": int(3),
			},
			expectResult: true,
		},
		{
			name:       "unsigned greater than unsigned",
			expression: `0x9 > x`,
			vars: VarMap{
				"x": uint(3),
			},
			expectResult: true,
		},
		{
			name:        "unsigned greater than string",
			expression:  `0x9 > "a"`,
			expectError: newLexerError(0, "rhs of > must be an integer"),
		},
		{
			name:         "negative",
			expression:   "-1",
			expectResult: int64(-1),
		},
		{
			name:         "signed equal 0",
			expression:   `5 == 0`,
			expectResult: false,
		},
		{
			name:         "signed not equal 0",
			expression:   `5 != 0`,
			expectResult: true,
		},
		{
			name:         "signed less than 0",
			expression:   `5 < 0`,
			expectResult: false,
		},
		{
			name:         "signed less than or equal 0",
			expression:   `0 <= 0`,
			expectResult: true,
		},
		{
			name:         "signed greater than 0",
			expression:   `5 > 0`,
			expectResult: true,
		},
		{
			name:         "signed greater than or equal 0",
			expression:   `5 >= 0`,
			expectResult: true,
		},
		{
			name:       "signed greater than unsigned",
			expression: `-1 > x`,
			vars: VarMap{
				"x": uint(3),
			},
			expectResult: false,
		},
		{
			name:        "signed greater than string",
			expression:  `-9 > "a"`,
			expectError: newLexerError(0, "rhs of > must be an integer"),
		},
		{
			name:       "negative unsigned var",
			expression: "-x",
			vars: VarMap{
				"x": uint64(3),
			},
			expectResult: int64(-3),
		},
		{
			name:       "negative signed var",
			expression: "-x",
			vars: VarMap{
				"x": int64(-3),
			},
			expectResult: int64(3),
		},
		{
			name:        "invalid negative",
			expression:  `-"abc"`,
			expectError: newLexerError(0, "rhs of - must be an integer"),
		},
		{
			name:        "failed to evaluate rhs",
			expression:  "0644 & unknown",
			expectError: newLexerError(7, `unknown variable "unknown"`),
		},
	}.Run(t)
}

func TestEvalBitOperations(t *testing.T) {
	instanceTests{
		{
			name:         "unsigned bitwise and",
			expression:   `0644 & 0647`,
			expectResult: uint64(0644),
		},
		{
			name:         "unsigned bitwise or",
			expression:   "0xbeef | 0xff",
			expectResult: uint64(0xbeff),
		},
		{
			name:         "unsigned bitwise xor",
			expression:   "0x0101 ^ 0x1010",
			expectResult: uint64(0x1111),
		},
		{
			name:         "unsigned unary bitwise not",
			expression:   "^0x0",
			expectResult: uint64(0xffffffffffffffff),
		},
		{
			name:        "unsigned bitwise and invalid rhs",
			expression:  `0644 & "abc"`,
			expectError: newLexerError(0, "rhs of & must be an integer"),
		},
		{
			name:       "signed bitwise and",
			expression: `x & y`,
			vars: VarMap{
				"x": int(1),
				"y": int(0),
			},
			expectResult: int64(0),
		},
		{
			name:       "signed bitwise or",
			expression: "x | y",
			vars: VarMap{
				"x": int(1),
				"y": int(0),
			},
			expectResult: int64(1),
		},
		{
			name:       "signed bitwise xor",
			expression: "x ^ y",
			vars: VarMap{
				"x": int(1),
				"y": int(0),
			},
			expectResult: int64(1),
		},
		{
			name:        "signed bitwise and invalid rhs",
			expression:  `0 & "abc"`,
			expectError: newLexerError(0, "rhs of & must be an integer"),
		},
		{
			name:       "signed unary bitwise not",
			expression: "^x",
			vars: VarMap{
				"x": -1,
			},
			expectResult: int64(0),
		},
		{
			name:       "signed and unsigned bitwise and",
			expression: `x & y`,
			vars: VarMap{
				"x": int(1),
				"y": uint(0),
			},
			expectResult: int64(0),
		},
		{
			name:       "signed and unsigned bitwise or",
			expression: "x | y",
			vars: VarMap{
				"x": int(1),
				"y": uint(0),
			},
			expectResult: int64(1),
		},
		{
			name:       "signed and unsigned bitwise xor",
			expression: "x ^ y",
			vars: VarMap{
				"x": int(1),
				"y": uint(0),
			},
			expectResult: int64(1),
		},
		{
			name:       "unsigned and signed bitwise and",
			expression: `x & y`,
			vars: VarMap{
				"x": uint(1),
				"y": int(0),
			},
			expectResult: uint64(0),
		},
		{
			name:       "unsigned and signed bitwise or",
			expression: "x | y",
			vars: VarMap{
				"x": uint(1),
				"y": int(0),
			},
			expectResult: uint64(1),
		},
		{
			name:       "unsigned and signed bitwise xor",
			expression: "x ^ y",
			vars: VarMap{
				"x": uint(1),
				"y": int(0),
			},
			expectResult: uint64(1),
		},
		{
			name:        "invalid unary bitwise not",
			expression:  `^"abc"`,
			expectError: newLexerError(0, "rhs of ^ must be an integer"),
		},
	}.Run(t)
}

func TestEvalString(t *testing.T) {
	instanceTests{
		{
			name:         "string equality",
			expression:   `"pong" == "pong"`,
			expectResult: true,
		},
		{
			name:         "string not equal",
			expression:   `"ping" != "pong"`,
			expectResult: true,
		},
		{
			name:         "string greater",
			expression:   `"abc" > "abb"`,
			expectResult: true,
		},
		{
			name:         "string less",
			expression:   `"abb" < "abc"`,
			expectResult: true,
		},
		{
			name:         "string greater or equal",
			expression:   `"abc" >= "abc"`,
			expectResult: true,
		},
		{
			name:         "string less or equal",
			expression:   `"abb" <= "abc"`,
			expectResult: true,
		},
		{
			name:         "string regexp match",
			expression:   `"abc" =~ "^a.+$"`,
			expectResult: true,
		},
		{
			name:         "string regexp not match",
			expression:   `"def" !~ "^a.+$"`,
			expectResult: true,
		},
		{
			name:         "string concat",
			expression:   `"abc" + "def"`,
			expectResult: "abcdef",
		},
		{
			name:        "invalid string concat",
			expression:  `"abc" + 0`,
			expectError: newLexerError(0, "rhs of + must be a string"),
		},
		{
			name:        "invalid string comparison",
			expression:  `"abc" > 0`,
			expectError: newLexerError(0, "rhs of > must be a string"),
		},
	}.Run(t)
}

func TestEvalArrayOperations(t *testing.T) {
	instanceTests{
		{
			name:         "in - string array - true",
			expression:   `"abc" in ["abc", "def"]`,
			expectResult: true,
		},
		{
			name:         "in - string array - false",
			expression:   `"xyz" in ["abc", "def"]`,
			expectResult: false,
		},
		{
			name:         "not in - string array - false",
			expression:   `"abc" not in ["abc", "def"]`,
			expectResult: false,
		},
		{
			name:         "not in - string array - true",
			expression:   `"xyz" not in ["abc", "def"]`,
			expectResult: true,
		},
		{
			name:         "in - mixed array - true",
			expression:   `"abc" in ["abc", 0]`,
			expectResult: true,
		},
		{
			name:         "in - mixed array - false",
			expression:   `0 in ["abc", 3]`,
			expectResult: false,
		},
		{
			name:       "in - var array - true",
			expression: "0 in zero",
			vars: VarMap{
				"zero": []interface{}{
					0,
				},
			},
			expectResult: true,
		},
		{
			name:       "invalid scalar array comparison",
			expression: "zero > -1",
			vars: VarMap{
				"zero": []interface{}{
					0,
				},
			},
			expectError: newLexerError(0, "lhs of > must be an integer or string"),
		},
		{
			name:       "invalid rhs of in",
			expression: "0 in notarray",
			vars: VarMap{
				"notarray": 0,
			},
			expectError: newLexerError(0, "rhs of in array operation must be an array"),
		},
	}.Run(t)
}

func TestEvalSubExpression(t *testing.T) {
	instanceTests{
		{
			name:         "boolean subexpression",
			expression:   `(3 > 5) || (4 == 4)`,
			expectResult: true,
		},
		{
			name:       "call subexpression",
			expression: "(4 == fn(2))",
			functions: FunctionMap{
				"fn": func(instance *Instance, args ...interface{}) (interface{}, error) {
					return 4, nil
				},
			},
			expectResult: true,
		},
	}.Run(t)
}

type iteratorMock struct {
	instances []*Instance
	index     int
}

func (i *iteratorMock) Next() (*Instance, error) {
	if !i.Done() {
		result := i.instances[i.index]
		i.index++
		return result, nil
	}
	return nil, errors.New("out of bounds iteration")
}

func (i *iteratorMock) Done() bool {
	return i.index >= len(i.instances)
}

func TestEvalIterable(t *testing.T) {
	instances := []*Instance{
		{
			Functions: map[string]Function{
				"has": func(instance *Instance, args ...interface{}) (interface{}, error) {
					return true, nil
				},
			},
			Vars: map[string]interface{}{
				"file.permissions": 0677,
				"file.owner":       "root",
			},
		},
		{
			Functions: map[string]Function{
				"has": func(instance *Instance, args ...interface{}) (interface{}, error) {
					return false, nil
				},
			},
			Vars: map[string]interface{}{
				"file.permissions": 0644,
				"file.owner":       "root",
			},
		},
		{
			Functions: map[string]Function{
				"has": func(instance *Instance, args ...interface{}) (interface{}, error) {
					return false, nil
				},
			},
			Vars: map[string]interface{}{
				"file.permissions": 0,
				"file.owner":       "root",
			},
		},
	}

	tests := []struct {
		name         string
		expression   string
		global       Instance
		expectResult bool
		expectError  error
	}{
		{
			name:         "len",
			expression:   `len(has("important-property") || file.permissions == 0644) == 2`,
			expectResult: true,
		},
		{
			name:        "len invalid comparison",
			expression:  `len(file.permissions == 0644) == "yes"`,
			expectError: newLexerError(0, `expecting an integer rhs for iterable comparison using len()`),
		},
		{
			name:        "len failed to evaluate rhs",
			expression:  `len(file.permissions == 0644) == EXPECTED`,
			expectError: newLexerError(33, `unknown variable "EXPECTED"`),
		},
		{
			name:         "all",
			expression:   `all(file.owner == "root")`,
			expectResult: true,
		},
		{
			name:         "none",
			expression:   `none(file.owner == "alice")`,
			expectResult: true,
		},
		{
			name:         "any",
			expression:   `any(file.owner == "alice")`,
			expectResult: false,
		},
		{
			name:         "no function",
			expression:   `file.owner == "alice"`,
			expectResult: false,
		},
		{
			name:        "unknown function",
			expression:  `some(file.owner == "alice")`,
			expectError: newLexerError(0, `unexpected function "some()" for iterable comparison`),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			iterator := &iteratorMock{
				instances: instances,
			}

			assert := assert.New(t)
			expr, err := ParseIterable(test.expression)
			assert.NoError(err)
			assert.NotNil(expr)

			value, err := expr.Evaluate(iterator, &test.global)
			if test.expectError != nil {
				assert.Equal(test.expectError, err)
			} else {
				assert.NoError(err)
				assert.Equal(test.expectResult, value.Passed)
			}
		})
	}
}

func TestEvalPathExpression(t *testing.T) {
	instance := &Instance{
		Functions: map[string]Function{
			"shell.command.stdout": func(instance *Instance, args ...interface{}) (interface{}, error) {
				return "/etc/path-from-command", nil
			},
			"process.flag": func(instance *Instance, args ...interface{}) (interface{}, error) {
				return "/etc/path-from-process", nil
			},
		},
	}

	tests := []struct {
		name       string
		expression string
		path       string
	}{
		{
			name:       "path",
			expression: `/etc/passwd`,
			path:       `/etc/passwd`,
		},
		{
			name:       "glob",
			expression: `/var/run/*.sock`,
			path:       `/var/run/*.sock`,
		},
		{
			name:       "path from command",
			expression: `shell.command.stdout("/usr/bin/find-my-path", "-v")`,
			path:       "/etc/path-from-command",
		},
		{
			name:       "path from process flag",
			expression: `process.flag("kubelet", "--config")`,
			path:       "/etc/path-from-process",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			assert := assert.New(t)
			expr, err := ParsePath(test.expression)
			assert.NoError(err)
			assert.NotNil(expr)

			value, err := expr.Evaluate(instance)
			assert.NoError(err)
			assert.Equal(test.path, value)
		})
	}
}
