package secl

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
	"github.com/alecthomas/participle/lexer"
)

func TestExprAt(t *testing.T) {
	rule, err := ast.ParseRule(`process.name != "/usr/bin/vipw" && open.pathname == "/etc/passwd" && (open.mode == O_TRUNC || open.mode == O_CREAT || open.mode == O_WRONLY)`)
	if err != nil {
		t.Error(err)
	}

	t.Log(SprintExprAt(rule.Expr, lexer.Position{Column: 22}))
}
