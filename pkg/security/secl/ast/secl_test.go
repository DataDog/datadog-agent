package ast

import (
	"encoding/json"
	"testing"

	"github.com/alecthomas/participle/lexer"
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
