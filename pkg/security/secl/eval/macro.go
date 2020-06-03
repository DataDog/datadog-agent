package eval

import (
	"github.com/DataDog/datadog-agent/pkg/security/policy"
	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
)

type Macro struct {
	ID         policy.MacroID
	Expression string
	ast        *ast.Macro
	// evaluator  *MacroEvaluator
}

func (m *Macro) Parse() error {
	astMacro, err := ast.ParseMacro(m.Expression)
	if err != nil {
		return err
	}
	m.ast = astMacro
	return nil
}
