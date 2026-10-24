// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"errors"
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/allowedpaths"
)

// validateNode walks the AST and rejects shell constructs that are not
// supported in the safe-shell interpreter.  It is called before execution
// so that disallowed features are caught early with a clear error message.
// remediationMode allows file-target output redirections (>, >>, 2>, &>, &>>)
// to pass validation; they are still gated by the sandbox at runtime.
func validateNode(node syntax.Node, remediationMode bool) error {
	var err error
	hdocWords := make(map[*syntax.Word]bool)
	syntax.Walk(node, func(n syntax.Node) bool {
		if err != nil {
			return false
		}
		switch n := n.(type) {
		// Blocked expression-level nodes.
		case *syntax.ArithmExp:
			err = errors.New("arithmetic expansion is not supported")
			return false
		case *syntax.ProcSubst:
			err = errors.New("process substitution is not supported")
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
		case *syntax.CaseClause:
			err = errors.New("case statements are not supported")
			return false
		case *syntax.FuncDecl:
			err = errors.New("function declarations are not supported")
			return false
		case *syntax.ArithmCmd:
			err = errors.New("arithmetic commands are not supported")
			return false
		case *syntax.TestClause:
			err = errors.New("test expressions are not supported")
			return false
		case *syntax.DeclClause:
			err = fmt.Errorf("%s is not supported", n.Variant.Value)
			return false
		case *syntax.LetClause:
			err = errors.New("let is not supported")
			return false
		case *syntax.TimeClause:
			err = errors.New("time is not supported")
			return false
		case *syntax.CoprocClause:
			err = errors.New("coprocesses are not supported")
			return false
		case *syntax.TestDecl:
			err = errors.New("test declarations are not supported")
			return false
		case *syntax.ForClause:
			if n.Select {
				err = errors.New("select statements are not supported")
				return false
			}
			if _, ok := n.Loop.(*syntax.WordIter); !ok {
				err = errors.New("c-style for loops are not supported")
				return false
			}
		case *syntax.ExtGlob:
			err = errors.New("extended globbing is not supported")
			return false

		// Blocked statement-level features.
		case *syntax.Stmt:
			if n.Background {
				err = errors.New("background execution (&) is not supported")
				return false
			}

		// Blocked pipe operators.
		case *syntax.BinaryCmd:
			if n.Op == syntax.PipeAll {
				err = errors.New("|& is not supported")
				return false
			}

		// Blocked redirections.
		case *syntax.Redirect:
			err = validateRedirect(n, remediationMode)
			if err != nil {
				return false
			}
			if n.Hdoc != nil {
				hdocWords[n.Hdoc] = true
			}

		// Blocked tilde expansion (prevents host user info disclosure via os/user.Lookup).
		// Heredoc bodies are excluded since they don't undergo tilde expansion.
		case *syntax.Word:
			if !hdocWords[n] && len(n.Parts) > 0 {
				if lit, ok := n.Parts[0].(*syntax.Lit); ok {
					if strings.HasPrefix(lit.Value, "~") {
						err = errors.New("tilde expansion is not supported")
						return false
					}
				}
			}

		// Explicitly allowed command-level nodes. These are the only
		// syntax.Command implementations that safe-shell permits.
		// Listing them here ensures any new Command type added by a future
		// version of mvdan.cc/sh/v3 is caught by the catch-all below.
		// NOTE: *syntax.BinaryCmd and *syntax.ForClause are handled by their
		// own cases above (with partial restrictions) and must not appear here.
		case *syntax.CallExpr, *syntax.IfClause, *syntax.WhileClause, *syntax.Block, *syntax.Subshell:
			// allowed — no action

		// Catch-all for unknown Command types not explicitly listed above.
		// This guards against new command node types added by upstream
		// library upgrades silently bypassing validation.
		case syntax.Command:
			err = fmt.Errorf("unsupported command type: %T", n)
			return false
		}
		return true
	})
	return err
}

// blockedSpecialParams are single-character parameter names that are not
// supported in the safe-shell interpreter (positional params, $#, $0, $@, $*).
var blockedSpecialParams = map[string]bool{
	"#": true,                                  // $# - number of positional parameters
	"!": true,                                  // $! - PID of the last background command
	"0": true,                                  // $0 - name of the shell or script
	"1": true, "2": true, "3": true, "4": true, // $1-$9 - positional parameters
	"5": true, "6": true, "7": true, "8": true, "9": true,
	"@": true, // $@ - all positional parameters as separate words
	"*": true, // $* - all positional parameters as a single word
}

func validateParamExp(pe *syntax.ParamExp) error {
	if pe.Length {
		return errors.New("${#var} is not supported")
	}
	if pe.Slice != nil {
		return errors.New("${var:offset} is not supported")
	}
	if pe.Repl != nil {
		return errors.New("${var/pattern/replacement} is not supported")
	}
	if pe.Excl {
		return errors.New("${!var} is not supported")
	}
	if pe.Index != nil {
		return errors.New("array indexing is not supported")
	}
	if pe.Names != 0 {
		return errors.New("${!prefix*} is not supported")
	}
	if pe.Exp != nil {
		return errors.New("${var} operations (defaults, pattern removal, case conversion) are not supported")
	}
	// Block special parameters like $#, $0, $1-$9, $@, $*
	if pe.Param != nil && blockedSpecialParams[pe.Param.Value] {
		return fmt.Errorf("$%s is not supported", pe.Param.Value)
	}
	if pe.Param != nil && pe.Param.Value == "LINENO" {
		return errors.New("$LINENO is not supported")
	}
	return nil
}

func validateAssign(as *syntax.Assign) error {
	if as.Append {
		return errors.New("+= is not supported")
	}
	if as.Array != nil {
		return errors.New("array assignment is not supported")
	}
	if as.Index != nil {
		return errors.New("array index assignment is not supported")
	}
	return nil
}

// redirectAllowed reports whether a file-target redirect passes static
// validation: either the target is a literal /dev/null (always accepted), the
// runner is in remediation mode (file writes enabled), or the target is not a
// static literal and must therefore be judged at run time.
//
// The validator deliberately does not resolve expansions — evaluating user
// input at validation time is exactly what this static gate exists to avoid.
// So a dynamic target (e.g. `> $F`) is deferred to the equally strict runtime
// check in (*Runner).redir, which inspects the expanded string and still
// permits only /dev/null in read-only mode. Deferring also gives a better
// failure mode: the offending command fails with exit 1 and the script keeps
// going, instead of the whole program being aborted with exit 2 before
// execution starts.
//
// A target containing a command substitution is the one dynamic form that is
// still rejected statically: deferring it would let the substituted command
// run before the redirect is refused, so the "a blocked redirect executes
// nothing" property is preserved by keeping that case at validation time.
//
// Extracted to avoid repeating the same condition across every write-redirect
// case in validateRedirect.
func redirectAllowed(rd *syntax.Redirect, remediationMode bool) bool {
	if redirectTargetIsDevNull(rd) || remediationMode {
		return true
	}
	return !redirectTargetIsStaticLiteral(rd) && !redirectTargetRunsCommands(rd)
}

// redirectTargetRunsCommands reports whether expanding the redirect word would
// execute a command (command or process substitution), anywhere in the word
// including inside a parameter expansion's default/alternate value.
func redirectTargetRunsCommands(rd *syntax.Redirect) bool {
	if rd.Word == nil {
		return false
	}
	found := false
	syntax.Walk(rd.Word, func(n syntax.Node) bool {
		switch n.(type) {
		case *syntax.CmdSubst, *syntax.ProcSubst:
			found = true
		}
		return !found
	})
	return found
}

// redirectTargetIsStaticLiteral reports whether the redirect word is a single
// unquoted literal, so its final value is fully determined by the AST and can
// be decided at validation time. Anything else — parameter expansions, quoted
// words, command substitutions, multi-part words — only has a value after
// expansion and is left to the runtime check.
func redirectTargetIsStaticLiteral(rd *syntax.Redirect) bool {
	if rd.Word == nil || len(rd.Word.Parts) != 1 {
		return false
	}
	_, ok := rd.Word.Parts[0].(*syntax.Lit)
	return ok
}

func validateRedirect(rd *syntax.Redirect, remediationMode bool) error {
	switch rd.Op {
	case syntax.WordHdoc:
		return errors.New("<<< (herestring) is not supported")
	case syntax.RdrIn:
		// Input redirection: only fd 0 (stdin) is supported.
		// rd.N is nil when no explicit fd is given (defaults to 0).
		if rd.N != nil && rd.N.Value != "0" {
			return fmt.Errorf("%s< input fd redirection is not supported", rd.N.Value)
		}
		return nil
	case syntax.RdrOut, syntax.RdrClob:
		// Only stdout (fd 1) and stderr (fd 2) are supported; rd.N is nil
		// when no explicit fd is given (defaults to 1).  Reject unsupported
		// fds here — before redirectAllowed — so that a command substitution
		// in the target (e.g. 3>$(evil)) cannot execute before the fd check.
		if rd.N != nil && rd.N.Value != "1" && rd.N.Value != "2" {
			return fmt.Errorf("%s> fd redirection is not supported", rd.N.Value)
		}
		if redirectAllowed(rd, remediationMode) {
			return nil
		}
		return errors.New("> file redirection is not supported")
	case syntax.AppOut:
		if rd.N != nil && rd.N.Value != "1" && rd.N.Value != "2" {
			return fmt.Errorf("%s>> fd redirection is not supported", rd.N.Value)
		}
		if redirectAllowed(rd, remediationMode) {
			return nil
		}
		return errors.New(">> file redirection is not supported")
	case syntax.RdrAll:
		if redirectAllowed(rd, remediationMode) {
			return nil
		}
		return errors.New("&> file redirection is not supported")
	case syntax.AppAll:
		if redirectAllowed(rd, remediationMode) {
			return nil
		}
		return errors.New("&>> file redirection is not supported")
	case syntax.RdrInOut:
		// <> (read-write open) is blocked in all modes.
		return errors.New("<> file redirection is not supported")
	case syntax.DplOut:
		if redirectTargetIsFD(rd) {
			return nil
		}
		return errors.New(">&N fd duplication is not supported")
	case syntax.DplIn:
		return errors.New("<&N fd duplication is not supported")
	}
	return nil
}

// redirectTargetIsDevNull reports whether the redirect word is the literal
// path /dev/null (or os.DevNull on Windows). Only simple literal words are
// accepted — variable expansions, globs, and other dynamic forms are rejected
// so that the target cannot be manipulated at runtime. The source fd (rd.N)
// must also be a supported fd (1 or 2).
func redirectTargetIsDevNull(rd *syntax.Redirect) bool {
	// Check source fd: only 1 (stdout) and 2 (stderr) are supported.
	// rd.N is nil when no explicit fd is given (defaults to stdout).
	// For RdrAll/AppAll (&>/&>>), rd.N is always nil since bash does
	// not allow an explicit fd prefix on these ops, so this check is
	// a no-op for them.
	if rd.N != nil && rd.N.Value != "1" && rd.N.Value != "2" {
		return false
	}
	if rd.Word == nil || len(rd.Word.Parts) != 1 {
		return false
	}
	lit, ok := rd.Word.Parts[0].(*syntax.Lit)
	if !ok {
		return false
	}
	return isDevNull(lit.Value)
}

// redirectTargetIsFD reports whether the DplOut (>&N) redirect uses only
// supported file descriptors (1 and 2 for stdout/stderr). Both the source
// fd (rd.N, defaulting to 1) and target fd (rd.Word) must be 1 or 2.
func redirectTargetIsFD(rd *syntax.Redirect) bool {
	// Check source fd (rd.N). If nil, defaults to 1 (stdout), which is fine.
	if rd.N != nil && rd.N.Value != "1" && rd.N.Value != "2" {
		return false
	}
	if rd.Word == nil || len(rd.Word.Parts) != 1 {
		return false
	}
	lit, ok := rd.Word.Parts[0].(*syntax.Lit)
	if !ok {
		return false
	}
	return lit.Value == "1" || lit.Value == "2"
}

// isDevNull delegates to allowedpaths.IsDevNull.
func isDevNull(path string) bool {
	return allowedpaths.IsDevNull(path)
}
