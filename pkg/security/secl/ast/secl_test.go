// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package ast

import (
	"encoding/json"
	"testing"
)

func print(t *testing.T, i interface{}) {
	b, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		t.Error(err)
	}

	t.Log(string(b))
}

func TestEmptyRule(t *testing.T) {
	_, err := ParseRule(``)
	if err == nil {
		t.Error("Empty expression should not be valid")
	}
}

func TestCompareNumbers(t *testing.T) {
	rule, err := ParseRule(`-3 > 1`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestCompareSimpleIdent(t *testing.T) {
	rule, err := ParseRule(`process > 1`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestCompareCompositeIdent(t *testing.T) {
	rule, err := ParseRule(`process.pid > 1`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestCompareString(t *testing.T) {
	rule, err := ParseRule(`process.name == "/usr/bin/ls"`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestCompareComplex(t *testing.T) {
	rule, err := ParseRule(`process.name != "/usr/bin/vipw" && open.pathname == "/etc/passwd" && (open.mode == O_TRUNC || open.mode == O_CREAT || open.mode == O_WRONLY)`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestRegister(t *testing.T) {
	rule, err := ParseRule(`process.ancestors[A].filename == "/usr/bin/vipw" && process.ancestors[A].pid == 44`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestBoolAnd(t *testing.T) {
	rule, err := ParseRule(`3 & 3`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestInArrayString(t *testing.T) {
	rule, err := ParseRule(`"a" in [ "a", "b", "c" ]`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestInArrayInteger(t *testing.T) {
	rule, err := ParseRule(`1 in [ 1, 2, 3 ]`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestMacroList(t *testing.T) {
	macro, err := ParseMacro(`[ 1, 2, 3 ]`)
	if err != nil {
		t.Error(err)
	}

	print(t, macro)
}

func TestMacroPrimary(t *testing.T) {
	macro, err := ParseMacro(`true`)
	if err != nil {
		t.Error(err)
	}

	print(t, macro)
}

func TestMacroExpression(t *testing.T) {
	macro, err := ParseMacro(`1 in [ 1, 2, 3 ]`)
	if err != nil {
		t.Error(err)
	}

	print(t, macro)
}

func TestMultiline(t *testing.T) {
	expr := `process.filename == "/usr/bin/vipw" &&
	process.pid == 44`

	if _, err := ParseRule(expr); err != nil {
		t.Error(err)
	}

	expr = `process.filename in ["/usr/bin/vipw",
	"/usr/bin/test"]`

	if _, err := ParseRule(expr); err != nil {
		t.Error(err)
	}

	expr = `process.filename == "/usr/bin/vipw" && (
	process.filename == "/usr/bin/test" ||
	process.filename == "/ust/bin/false"
	)`

	if _, err := ParseRule(expr); err != nil {
		t.Error(err)
	}
}
