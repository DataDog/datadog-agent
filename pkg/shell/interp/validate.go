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
		}
		return true
	})
	return err
}

// blockedSpecialParams are single-character parameter names that are not
// supported in the safe-shell interpreter (positional params, $#, $0, $@, $*).
var blockedSpecialParams = map[string]bool{
	"#": true,
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
