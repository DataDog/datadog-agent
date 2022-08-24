// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

func TestIntAnd(t *testing.T) {
	rule, err := ParseRule(`3 & 3`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestBoolAnd(t *testing.T) {
	rule, err := ParseRule(`true and true`)
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
	process.filename == "/usr/bin/test" || # blah blah
	# blah blah
	process.filename == "/ust/bin/false"
	)`

	if _, err := ParseRule(expr); err != nil {
		t.Error(err)
	}
}

func TestPattern(t *testing.T) {
	rule, err := ParseRule(`process.name == ~"/usr/bin/ls"`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestArrayPattern(t *testing.T) {
	rule, err := ParseRule(`process.name in [~"/usr/bin/ls", "/usr/sbin/ls"]`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestRegexp(t *testing.T) {
	rule, err := ParseRule(`process.name == r"/usr/bin/ls"`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)

	rule, err = ParseRule(`process.name == r"^((?:[A-Za-z\d+]{4})*(?:[A-Za-z\d+]{3}=|[A-Za-z\d+]{2}==)\.)*(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z1-9])$" `)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestArrayRegexp(t *testing.T) {
	rule, err := ParseRule(`process.name in [r"/usr/bin/ls", "/usr/sbin/ls"]`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestDuration(t *testing.T) {
	rule, err := ParseRule(`process.start > 10s`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestNumberVariable(t *testing.T) {
	rule, err := ParseRule(`process.pid == ${pid}`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestIPv4(t *testing.T) {
	rule, err := ParseRule(`network.source.ip == 127.0.0.1`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestIPv4Raw(t *testing.T) {
	rule, err := ParseRule(`127.0.0.2 == 127.0.0.1`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestIPv6Localhost(t *testing.T) {
	rule, err := ParseRule(`network.source.ip == ::1`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestIPv6(t *testing.T) {
	rule, err := ParseRule(`network.source.ip == 2001:0000:0eab:DEAD:0000:00A0:ABCD:004E`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestIPv6Short(t *testing.T) {
	rule, err := ParseRule(`network.source.ip == 2001:0:0eab:dead::a0:abcd:4e`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestIPArray(t *testing.T) {
	rule, err := ParseRule(`network.source.ip in [ ::1, 2001:0:0eab:dead::a0:abcd:4e, 127.0.0.1 ]`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestIPv4CIDR(t *testing.T) {
	rule, err := ParseRule(`network.source.ip in 192.168.0.0/24`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestIPv6CIDR(t *testing.T) {
	rule, err := ParseRule(`network.source.ip in ::1/128`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestCIDRArray(t *testing.T) {
	rule, err := ParseRule(`network.source.ip in [ ::1/128, 2001:0:0eab:dead::a0:abcd:4e/24, 127.0.0.1/32 ]`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestIPAndCIDRArray(t *testing.T) {
	rule, err := ParseRule(`network.source.ip in [ ::1, 2001:0:0eab:dead::a0:abcd:4e, 127.0.0.1, ::1/128, 2001:0:0eab:dead::a0:abcd:4e/24, 127.0.0.1/32 ]`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestCIDRMatches(t *testing.T) {
	rule, err := ParseRule(`network.source.cidr allin 192.168.0.0/24`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}

func TestCIDRArrayMatches(t *testing.T) {
	rule, err := ParseRule(`network.source.cidr allin [ 192.168.0.0/24, ::1/128 ]`)
	if err != nil {
		t.Error(err)
	}

	print(t, rule)
}
