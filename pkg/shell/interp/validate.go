// Copyright (c) Datadog, Inc.
// See LICENSE for licensing information

package interp

import (
	"fmt"

	"mvdan.cc/sh/v3/syntax"
)

// validateNode walks the AST and rejects shell constructs that are not
// supported in the safe-shell interpreter.  It is called before execution
// so that disallowed features are caught early with a clear error message.
func validateNode(node syntax.Node) error {
	var err error
	syntax.Walk(node, func(n syntax.Node) bool {
		if err != nil {
			return false
		}
		switch n := n.(type) {
		// Blocked expression-level nodes.
		case *syntax.ArithmExp:
			err = fmt.Errorf("arithmetic expansion is not supported")
			return false
		case *syntax.CmdSubst:
			err = fmt.Errorf("command substitution is not supported")
			return false
		case *syntax.ProcSubst:
			err = fmt.Errorf("process substitution is not supported")
			return false
		case *syntax.ParamExp:
			err = validateParamExp(n)
			if err != nil {
				return false
			}
		case *syntax.Assign:
			err = validateAssign(n)
			if err != nil {
				return false
			}

		// Blocked command-level nodes.
		case *syntax.IfClause:
			err = fmt.Errorf("if statements are not supported")
			return false
		case *syntax.WhileClause:
			err = fmt.Errorf("while/until loops are not supported")
			return false
		case *syntax.CaseClause:
			err = fmt.Errorf("case statements are not supported")
			return false
		case *syntax.Subshell:
			err = fmt.Errorf("subshells are not supported")
			return false
		case *syntax.FuncDecl:
			err = fmt.Errorf("function declarations are not supported")
			return false
		case *syntax.ArithmCmd:
			err = fmt.Errorf("arithmetic commands are not supported")
			return false
		case *syntax.TestClause:
			err = fmt.Errorf("test expressions are not supported")
			return false
		case *syntax.DeclClause:
			err = fmt.Errorf("%s is not supported", n.Variant.Value)
			return false
		case *syntax.LetClause:
			err = fmt.Errorf("let is not supported")
			return false
		case *syntax.TimeClause:
			err = fmt.Errorf("time is not supported")
			return false
		case *syntax.CoprocClause:
			err = fmt.Errorf("coprocesses are not supported")
			return false
		case *syntax.TestDecl:
			err = fmt.Errorf("test declarations are not supported")
			return false
		case *syntax.ForClause:
			if n.Select {
				err = fmt.Errorf("select statements are not supported")
				return false
			}
			if _, ok := n.Loop.(*syntax.WordIter); !ok {
				err = fmt.Errorf("c-style for loops are not supported")
				return false
			}
		case *syntax.ExtGlob:
			err = fmt.Errorf("extended globbing is not supported")
			return false

		// Blocked statement-level features.
		case *syntax.Stmt:
			if n.Background {
				err = fmt.Errorf("background execution (&) is not supported")
				return false
			}

		// Blocked redirections.
		case *syntax.Redirect:
			err = validateRedirect(n)
			if err != nil {
				return false
			}
		}
		return true
	})
	return err
}

// blockedSpecialParams are single-character parameter names that are not
// supported in the safe-shell interpreter (positional params, $#, $0, $@, $*).
var blockedSpecialParams = map[string]bool{
	"#": true,
	"!": true,
	"0": true,
	"1": true, "2": true, "3": true, "4": true,
	"5": true, "6": true, "7": true, "8": true, "9": true,
	"@": true,
	"*": true,
}

func validateParamExp(pe *syntax.ParamExp) error {
	if pe.Length {
		return fmt.Errorf("${#var} is not supported")
	}
	if pe.Slice != nil {
		return fmt.Errorf("${var:offset} is not supported")
	}
	if pe.Repl != nil {
		return fmt.Errorf("${var/pattern/replacement} is not supported")
	}
	if pe.Excl {
		return fmt.Errorf("${!var} is not supported")
	}
	if pe.Index != nil {
		return fmt.Errorf("array indexing is not supported")
	}
	if pe.Names != 0 {
		return fmt.Errorf("${!prefix*} is not supported")
	}
	if pe.Exp != nil {
		return fmt.Errorf("${var} operations (defaults, pattern removal, case conversion) are not supported")
	}
	// Block special parameters like $#, $0, $1-$9, $@, $*
	if pe.Param != nil && blockedSpecialParams[pe.Param.Value] {
		return fmt.Errorf("$%s is not supported", pe.Param.Value)
	}
	if pe.Param != nil && pe.Param.Value == "LINENO" {
		return fmt.Errorf("$LINENO is not supported")
	}
	return nil
}

func validateAssign(as *syntax.Assign) error {
	if as.Append {
		return fmt.Errorf("+= is not supported")
	}
	if as.Array != nil {
		return fmt.Errorf("array assignment is not supported")
	}
	if as.Index != nil {
		return fmt.Errorf("array index assignment is not supported")
	}
	return nil
}

func validateRedirect(rd *syntax.Redirect) error {
	switch rd.Op {
	case syntax.RdrOut, syntax.ClbOut:
		return fmt.Errorf("> file redirection is not supported")
	case syntax.AppOut:
		return fmt.Errorf(">> file redirection is not supported")
	case syntax.RdrAll:
		return fmt.Errorf("&> file redirection is not supported")
	case syntax.AppAll:
		return fmt.Errorf("&>> file redirection is not supported")
	case syntax.RdrInOut:
		return fmt.Errorf("<> file redirection is not supported")
	case syntax.DplOut:
		return fmt.Errorf(">&N fd duplication is not supported")
	case syntax.DplIn:
		return fmt.Errorf("<&N fd duplication is not supported")
	}
	return nil
}
