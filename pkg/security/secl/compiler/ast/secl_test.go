// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ast holds ast related files
package ast

import (
	"encoding/json"
	"testing"
)

func parseRule(rule string) (*Rule, error) {
	pc := NewParsingContext(false)
	return pc.ParseRule(rule)
}

func parseMacro(macro string) (*Macro, error) {
	pc := NewParsingContext(false)
	return pc.ParseMacro(macro)
}

func printJSON(t *testing.T, i interface{}) {
	b, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		t.Error(err)
	}

	t.Log(string(b))
}

func TestEmptyRule(t *testing.T) {
	_, err := parseRule(``)
	if err == nil {
		t.Error("Empty expression should not be valid")
	}
}

func TestCompareNumbers(t *testing.T) {
	rule, err := parseRule(`-3 > 1`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestCompareSimpleIdent(t *testing.T) {
	rule, err := parseRule(`process > 1`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestCompareCompositeIdent(t *testing.T) {
	rule, err := parseRule(`process.pid > 1`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestCompareString(t *testing.T) {
	rule, err := parseRule(`process.name == "/usr/bin/ls"`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestCompareComplex(t *testing.T) {
	rule, err := parseRule(`process.name != "/usr/bin/vipw" && open.pathname == "/etc/passwd" && (open.mode == O_TRUNC || open.mode == O_CREAT || open.mode == O_WRONLY)`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestRegister(t *testing.T) {
	rule, err := parseRule(`process.ancestors[A].filename == "/usr/bin/vipw" && process.ancestors[A].pid == 44`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestIntAnd(t *testing.T) {
	rule, err := parseRule(`3 & 3`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestBoolAnd(t *testing.T) {
	rule, err := parseRule(`true and true`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestInArrayString(t *testing.T) {
	rule, err := parseRule(`"a" in [ "a", "b", "c" ]`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestInArrayInteger(t *testing.T) {
	rule, err := parseRule(`1 in [ 1, 2, 3 ]`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestMacroList(t *testing.T) {
	macro, err := parseMacro(`[ 1, 2, 3 ]`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, macro)
}

func TestMacroPrimary(t *testing.T) {
	macro, err := parseMacro(`true`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, macro)
}

func TestMacroExpression(t *testing.T) {
	macro, err := parseMacro(`1 in [ 1, 2, 3 ]`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, macro)
}

func TestMultiline(t *testing.T) {
	expr := `process.filename == "/usr/bin/vipw" &&
	process.pid == 44`

	if _, err := parseRule(expr); err != nil {
		t.Error(err)
	}

	expr = `process.filename in ["/usr/bin/vipw",
	"/usr/bin/test"]`

	if _, err := parseRule(expr); err != nil {
		t.Error(err)
	}

	expr = `process.filename == "/usr/bin/vipw" && (
	process.filename == "/usr/bin/test" || # blah blah
	# blah blah
	process.filename == "/ust/bin/false"
	)`

	if _, err := parseRule(expr); err != nil {
		t.Error(err)
	}
}

func TestPattern(t *testing.T) {
	rule, err := parseRule(`process.name == ~"/usr/bin/ls"`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestArrayPattern(t *testing.T) {
	rule, err := parseRule(`process.name in [~"/usr/bin/ls", "/usr/sbin/ls"]`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestRegexp(t *testing.T) {
	rule, err := parseRule(`process.name == r"/usr/bin/ls"`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)

	rule, err = parseRule(`process.name == r"^((?:[A-Za-z\d+]{4})*(?:[A-Za-z\d+]{3}=|[A-Za-z\d+]{2}==)\.)*(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z1-9])$" `)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestArrayRegexp(t *testing.T) {
	rule, err := parseRule(`process.name in [r"/usr/bin/ls", "/usr/sbin/ls"]`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestDuration(t *testing.T) {
	rule, err := parseRule(`process.start > 10s`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestNumberVariable(t *testing.T) {
	rule, err := parseRule(`process.pid == ${pid}`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestIPv4(t *testing.T) {
	rule, err := parseRule(`network.source.ip == 127.0.0.1`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestIPv4Raw(t *testing.T) {
	rule, err := parseRule(`127.0.0.2 == 127.0.0.1`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestIPv6Localhost(t *testing.T) {
	rule, err := parseRule(`network.source.ip == ::1`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestIPv6(t *testing.T) {
	rule, err := parseRule(`network.source.ip == 2001:0000:0eab:DEAD:0000:00A0:ABCD:004E`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestIPv6Short(t *testing.T) {
	rule, err := parseRule(`network.source.ip == 2001:0:0eab:dead::a0:abcd:4e`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestIPArray(t *testing.T) {
	rule, err := parseRule(`network.source.ip in [ ::1, 2001:0:0eab:dead::a0:abcd:4e, 127.0.0.1 ]`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestIPv4CIDR(t *testing.T) {
	rule, err := parseRule(`network.source.ip in 192.168.0.0/24`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestIPv6CIDR(t *testing.T) {
	rule, err := parseRule(`network.source.ip in ::1/128`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestCIDRArray(t *testing.T) {
	rule, err := parseRule(`network.source.ip in [ ::1/128, 2001:0:0eab:dead::a0:abcd:4e/24, 127.0.0.1/32 ]`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestIPAndCIDRArray(t *testing.T) {
	rule, err := parseRule(`network.source.ip in [ ::1, 2001:0:0eab:dead::a0:abcd:4e, 127.0.0.1, ::1/128, 2001:0:0eab:dead::a0:abcd:4e/24, 127.0.0.1/32 ]`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestCIDRMatches(t *testing.T) {
	rule, err := parseRule(`network.source.cidr allin 192.168.0.0/24`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}

func TestCIDRArrayMatches(t *testing.T) {
	rule, err := parseRule(`network.source.cidr allin [ 192.168.0.0/24, ::1/128 ]`)
	if err != nil {
		t.Error(err)
	}

	printJSON(t, rule)
}
