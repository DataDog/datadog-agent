// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package verifier

import (
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// verifyFile walks all statements in a parsed shell file.
func (v *verifier) verifyFile(f *syntax.File) {
	for _, stmt := range f.Stmts {
		v.verifyStmt(stmt)
	}
}

// verifyStmt checks a single statement and its command.
func (v *verifier) verifyStmt(stmt *syntax.Stmt) {
	if stmt == nil {
		return
	}

	// Reject background execution (&)
	if stmt.Background {
		v.addViolation(stmt.Pos(), "shell_feature", "background execution (&) is not allowed")
	}

	// Reject coprocess
	if stmt.Coprocess {
		v.addViolation(stmt.Pos(), "shell_feature", "coprocess is not allowed")
	}

	// Reject ALL redirections
	if len(stmt.Redirs) > 0 {
		v.addViolation(stmt.Redirs[0].Pos(), "redirect", "redirections are not allowed")
	}

	// Verify the command itself
	if stmt.Cmd != nil {
		v.verifyCommand(stmt.Cmd)
	}
}

// verifyCommand dispatches on the command node type.
func (v *verifier) verifyCommand(cmd syntax.Command) {
	switch c := cmd.(type) {
	case *syntax.CallExpr:
		v.verifyCallExpr(c)

	case *syntax.BinaryCmd:
		v.verifyBinaryCmd(c)

	case *syntax.Block:
		// { ... } compound commands — recurse into statements
		for _, stmt := range c.Stmts {
			v.verifyStmt(stmt)
		}

	case *syntax.IfClause:
		v.verifyIfClause(c)

	case *syntax.WhileClause:
		v.verifyWhileClause(c)

	case *syntax.ForClause:
		v.verifyForClause(c)

	case *syntax.CaseClause:
		v.verifyCaseClause(c)

	case *syntax.Subshell:
		v.addViolation(c.Pos(), "shell_feature", "subshell (...) is not allowed")

	case *syntax.FuncDecl:
		v.addViolation(c.Pos(), "shell_feature", "function declarations are not allowed")

	case *syntax.ArithmCmd:
		v.addViolation(c.Pos(), "shell_feature", "standalone arithmetic command (( )) is not allowed")

	case *syntax.TestClause:
		// [[ ... ]] conditionals — verify word parts in the test expression
		v.verifyTestExpr(c.X)

	case *syntax.DeclClause:
		// declare, local, export, readonly — verify the arguments
		v.verifyDeclClause(c)

	case *syntax.LetClause:
		v.addViolation(c.Pos(), "shell_feature", "let command is not allowed")

	case *syntax.TimeClause:
		v.addViolation(c.Pos(), "shell_feature", "time command is not allowed")

	case *syntax.CoprocClause:
		v.addViolation(c.Pos(), "shell_feature", "coproc is not allowed")

	default:
		v.addViolation(cmd.Pos(), "shell_feature", "unsupported command type")
	}
}

// verifyCallExpr verifies a simple command (command name + arguments).
func (v *verifier) verifyCallExpr(c *syntax.CallExpr) {
	if c == nil {
		return
	}

	// Verify all words for disallowed constructs (command substitution, etc.)
	for _, w := range c.Args {
		v.verifyWord(w)
	}

	// Verify assigns (e.g., VAR=value cmd)
	for _, assign := range c.Assigns {
		if assign.Index != nil {
			v.verifyArithmExpr(assign.Index)
		}
		if assign.Value != nil {
			v.verifyWord(assign.Value)
		}
		if assign.Array != nil {
			for _, elem := range assign.Array.Elems {
				if elem.Index != nil {
					v.verifyArithmExpr(elem.Index)
				}
				v.verifyWord(elem.Value)
			}
		}
	}

	// If there are no args, this is a pure assignment (VAR=value) — allowed.
	if len(c.Args) == 0 {
		return
	}

	// Block dangerous environment variable prefix assignments (e.g., PATH=/evil cmd).
	if len(c.Args) > 0 {
		for _, assign := range c.Assigns {
			if assign.Name != nil && dangerousEnvVars[assign.Name.Value] {
				v.addViolation(assign.Pos(), "shell_feature",
					fmt.Sprintf("setting %q in a command prefix assignment is not allowed", assign.Name.Value))
			}
		}
	}

	// The first arg is the command name — it must be a literal string.
	cmdWord := c.Args[0]
	cmdName, ok := literalWordValue(cmdWord)
	if !ok {
		v.addViolation(cmdWord.Pos(), "command", "command name must be a literal string (dynamic command names are not allowed)")
		return
	}

	// Check for explicitly blocked builtins.
	if blockedBuiltins[cmdName] {
		v.addViolation(cmdWord.Pos(), "command", fmt.Sprintf("command %q is not allowed", cmdName))
		return
	}

	// Check that the command is in the allowlist.
	allowedFlags, isAllowed := allowedCommands[cmdName]
	if !isAllowed {
		v.addViolation(cmdWord.Pos(), "command", fmt.Sprintf("command %q is not allowed", cmdName))
		return
	}

	// Verify flags against the per-command allowlist.
	// The test/[ builtins don't use conventional flags, so skip flag checking.
	if cmdName == "test" || cmdName == "[" {
		return
	}

	v.verifyFlags(cmdWord.Pos(), cmdName, c.Args[1:], allowedFlags)

	// Special handling: sed scripts need content analysis.
	if cmdName == "sed" {
		v.verifySedArgs(c.Args[1:])
	}
}

// verifyFlags checks that all flags used in a command are in the allowlist.
func (v *verifier) verifyFlags(pos syntax.Pos, cmdName string, args []*syntax.Word, allowed map[string]bool) {
	for _, arg := range args {
		val, ok := literalWordValue(arg)
		if !ok {
			// Non-literal argument — can't verify flags, but we've already
			// checked for command substitution etc. in verifyWord.
			continue
		}

		// Skip non-flag arguments (don't start with - or +)
		if !strings.HasPrefix(val, "-") && !strings.HasPrefix(val, "+") {
			continue
		}

		// Handle + prefixed flags (e.g., set +e, set +u) — check against allowlist.
		if strings.HasPrefix(val, "+") {
			if !allowed[val] {
				v.addViolation(arg.Pos(), "flag", fmt.Sprintf("flag %q is not allowed for command %q", val, cmdName))
			}
			continue
		}

		// Handle "--" (end of flags marker)
		if val == "--" {
			return
		}

		// Handle long flags (--foo or --foo=bar)
		if strings.HasPrefix(val, "--") {
			flagName := val
			if idx := strings.Index(val, "="); idx != -1 {
				flagName = val[:idx]
			}
			if !allowed[flagName] {
				v.addViolation(arg.Pos(), "flag", fmt.Sprintf("flag %q is not allowed for command %q", flagName, cmdName))
			}
			continue
		}

		// Handle combined short flags (e.g., -la → -l + -a)
		// But first check if the whole thing is a known flag.
		if allowed[val] {
			continue
		}

		// Check combined short flags character by character (e.g., -la → -l + -a).
		// When we hit a digit, the rest is a numeric value for the preceding flag,
		// BUT we must ensure ALL characters after the digits are also digits
		// (to prevent -n10f from sneaking through a blocked -f).
		for i := 1; i < len(val); i++ {
			singleFlag := "-" + string(val[i])
			if !allowed[singleFlag] {
				if val[i] >= '0' && val[i] <= '9' {
					// Verify the remainder is entirely digits (a flag value).
					allDigits := true
					for j := i; j < len(val); j++ {
						if val[j] < '0' || val[j] > '9' {
							allDigits = false
							break
						}
					}
					if allDigits {
						break // rest is numeric value — safe
					}
				}
				v.addViolation(arg.Pos(), "flag", fmt.Sprintf("flag %q is not allowed for command %q", singleFlag, cmdName))
				break
			}
		}
	}
}

// verifyBinaryCmd checks binary commands (&&, ||, |, |&).
func (v *verifier) verifyBinaryCmd(c *syntax.BinaryCmd) {
	if c == nil {
		return
	}

	switch c.Op {
	case syntax.AndStmt, syntax.OrStmt, syntax.Pipe:
		// &&, ||, | — allowed
	case syntax.PipeAll:
		// |& — rejected (stderr routing)
		v.addViolation(c.Pos(), "shell_feature", "pipe with stderr (|&) is not allowed")
	}

	v.verifyStmt(c.X)
	v.verifyStmt(c.Y)
}

// verifyIfClause walks if/elif/else blocks.
func (v *verifier) verifyIfClause(c *syntax.IfClause) {
	if c == nil {
		return
	}

	for _, stmt := range c.Cond {
		v.verifyStmt(stmt)
	}
	for _, stmt := range c.Then {
		v.verifyStmt(stmt)
	}
	if c.Else != nil {
		v.verifyIfClause(c.Else)
	}
	// The Else field doubles as the body for the final else block.
	// When Else.Cond is empty, it's a plain else (no elif).
}

// verifyWhileClause walks while/until loops.
func (v *verifier) verifyWhileClause(c *syntax.WhileClause) {
	if c == nil {
		return
	}
	for _, stmt := range c.Cond {
		v.verifyStmt(stmt)
	}
	for _, stmt := range c.Do {
		v.verifyStmt(stmt)
	}
}

// verifyForClause verifies for loops.
func (v *verifier) verifyForClause(c *syntax.ForClause) {
	if c == nil {
		return
	}

	switch loop := c.Loop.(type) {
	case *syntax.WordIter:
		// for i in ...; do ... done — allowed
		for _, w := range loop.Items {
			v.verifyWord(w)
		}
	case *syntax.CStyleLoop:
		// for ((i=0; i<10; i++)) — rejected (uses arithmetic commands)
		v.addViolation(c.Pos(), "shell_feature", "C-style for loop is not allowed")
		return
	default:
		v.addViolation(c.Pos(), "shell_feature", "unsupported for loop type")
		return
	}

	// Check if this is actually a select statement
	if c.Select {
		v.addViolation(c.Pos(), "shell_feature", "select statement is not allowed")
		return
	}

	for _, stmt := range c.Do {
		v.verifyStmt(stmt)
	}
}

// verifyCaseClause walks case statements.
func (v *verifier) verifyCaseClause(c *syntax.CaseClause) {
	if c == nil {
		return
	}

	v.verifyWord(c.Word)

	for _, item := range c.Items {
		for _, pattern := range item.Patterns {
			v.verifyWord(pattern)
		}
		for _, stmt := range item.Stmts {
			v.verifyStmt(stmt)
		}
	}
}

// verifyTestExpr walks a test expression ([[ ... ]]).
func (v *verifier) verifyTestExpr(expr syntax.TestExpr) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *syntax.BinaryTest:
		v.verifyTestExpr(e.X)
		v.verifyTestExpr(e.Y)
	case *syntax.UnaryTest:
		v.verifyTestExpr(e.X)
	case *syntax.ParenTest:
		v.verifyTestExpr(e.X)
	case *syntax.Word:
		v.verifyWord(e)
	}
}

// verifyDeclClause verifies declare/local/export/readonly commands.
func (v *verifier) verifyDeclClause(c *syntax.DeclClause) {
	if c == nil {
		return
	}

	// Validate the variant is one we explicitly allow.
	// typeset and nameref are dangerous: nameref enables indirect variable
	// manipulation that could be used for exploitation.
	switch c.Variant.Value {
	case "declare", "local", "export", "readonly":
		// allowed
	default:
		v.addViolation(c.Pos(), "command",
			fmt.Sprintf("declaration command %q is not allowed", c.Variant.Value))
		return
	}

	// Validate flags. In DeclClause, flags appear as Assign entries with Naked=true.
	cmdName := c.Variant.Value
	allowedFlags := allowedCommands[cmdName]
	for _, assign := range c.Args {
		if assign.Naked && assign.Value != nil {
			flagVal, ok := literalWordValue(assign.Value)
			if ok && strings.HasPrefix(flagVal, "-") {
				if !allowedFlags[flagVal] {
					v.addViolation(assign.Pos(), "flag",
						fmt.Sprintf("flag %q is not allowed for command %q", flagVal, cmdName))
				}
			}
		}
		// Walk Index, Value, and Array elements for command substitutions.
		if assign.Index != nil {
			v.verifyArithmExpr(assign.Index)
		}
		if assign.Value != nil {
			v.verifyWord(assign.Value)
		}
		if assign.Array != nil {
			for _, elem := range assign.Array.Elems {
				if elem.Index != nil {
					v.verifyArithmExpr(elem.Index)
				}
				v.verifyWord(elem.Value)
			}
		}
	}
}
