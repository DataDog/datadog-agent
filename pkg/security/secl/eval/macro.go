package eval

import "github.com/DataDog/datadog-agent/pkg/security/secl/ast"

type Macro struct {
	ID         string
	Expression string
	ast *ast.Macro
}

func (m *Macro) LoadAST() error {
	astMacro, err := ast.ParseMacro(m.Expression)
	if err != nil {
		return err
	}
	m.ast = astMacro
	return nil
}
