package dot

import (
	"os"
	"testing"

	"gitlab.com/safchain/secpoc/pkg/secl/ast"
)

func TestDotWriterParenthesis(t *testing.T) {
	rule, err := ast.ParseRule(`(1) == (1)`)
	if err != nil {
		t.Error(err)
	}

	dotMarshaller := NewDotMarshaler(os.Stdout)

	if err := dotMarshaller.MarshalRule(rule); err != nil {
		t.Error(err)
	}
}

func TestDotWriterInArray(t *testing.T) {
	rule, err := ast.ParseRule(`3 in [1, 2, 3]`)
	if err != nil {
		t.Error(err)
	}

	dotMarshaller := NewDotMarshaler(os.Stdout)

	if err := dotMarshaller.MarshalRule(rule); err != nil {
		t.Error(err)
	}
}
