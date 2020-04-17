package ast

import (
	"encoding/json"
	"testing"

	"github.com/alecthomas/participle/lexer"
)

func printRule(t *testing.T, r *Rule) {
	b, err := json.MarshalIndent(r, "", "  ")
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

	printRule(t, rule)
}

func TestCompareSimpleIdent(t *testing.T) {
	rule, err := ParseRule(`process > 1`)
	if err != nil {
		t.Error(err)
	}

	printRule(t, rule)
}

func TestCompareCompositeIdent(t *testing.T) {
	rule, err := ParseRule(`process.pid > 1`)
	if err != nil {
		t.Error(err)
	}

	printRule(t, rule)
}

func TestCompareString(t *testing.T) {
	rule, err := ParseRule(`process.name == "/usr/bin/ls"`)
	if err != nil {
		t.Error(err)
	}

	printRule(t, rule)
}

func TestCompareComplex(t *testing.T) {
	rule, err := ParseRule(`process.name != "/usr/bin/vipw" && open.pathname == "/etc/passwd" && (open.mode == O_TRUNC || open.mode == O_CREAT || open.mode == O_WRONLY)`)
	if err != nil {
		t.Error(err)
	}

	printRule(t, rule)
}
func TestExprAt(t *testing.T) {
	rule, err := ParseRule(`process.name != "/usr/bin/vipw" && open.pathname == "/etc/passwd" && (open.mode == O_TRUNC || open.mode == O_CREAT || open.mode == O_WRONLY)`)
	if err != nil {
		t.Error(err)
	}

	t.Log(rule.ExprAt(lexer.Position{Column: 22}))
}

func TestBoolAnd(t *testing.T) {
	rule, err := ParseRule(`3 & 3`)
	if err != nil {
		t.Error(err)
	}

	printRule(t, rule)
}

func TestInArrayString(t *testing.T) {
	rule, err := ParseRule(`"a" in [ "a", "b", "c" ]`)
	if err != nil {
		t.Error(err)
	}

	printRule(t, rule)
}

func TestInArrayInteger(t *testing.T) {
	rule, err := ParseRule(`1 in [ 1, 2, 3 ]`)
	if err != nil {
		t.Error(err)
	}

	printRule(t, rule)
}
