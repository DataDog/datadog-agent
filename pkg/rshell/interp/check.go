// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/allowedpaths"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// CheckStatus describes a static authorization result, not an exit status.
type CheckStatus string

const (
	CheckAllowed       CheckStatus = "allowed"
	CheckDenied        CheckStatus = "denied"
	CheckIndeterminate CheckStatus = "indeterminate"
)

// CheckIssueCode is a machine-readable explanation of a check result.
type CheckIssueCode string

const (
	CheckParseError              CheckIssueCode = "parse_error"
	CheckUnsupportedSyntax       CheckIssueCode = "unsupported_syntax"
	CheckCommandNotAllowed       CheckIssueCode = "command_not_allowed"
	CheckUnknownCommand          CheckIssueCode = "unknown_command"
	CheckPathNotAllowed          CheckIssueCode = "path_not_allowed"
	CheckRemediationRequired     CheckIssueCode = "remediation_required"
	CheckElevationNotAllowed     CheckIssueCode = "elevation_not_allowed"
	CheckSystemServiceNotAllowed CheckIssueCode = "system_service_not_allowed"
	CheckInvalidArguments        CheckIssueCode = "invalid_arguments"
	CheckReadonlyVariable        CheckIssueCode = "readonly_variable"
	CheckRequiresExecution       CheckIssueCode = "requires_execution"
	CheckLimitExceeded           CheckIssueCode = "limit_exceeded"
)

// CheckIssue explains a denial or an incomplete check. Path is populated for
// path-policy failures and contains the requested path, not a resolved target.
type CheckIssue struct {
	Code    CheckIssueCode `json:"code"`
	Message string         `json:"message"`
	Path    string         `json:"path,omitempty"`
}

// CommandCheck describes one syntactic command site, including commands in
// substitutions, pipelines, and conditional branches. Line and Column are
// one-based. Command is the expanded command name, without a sudo marker; it
// is empty for assignments, redirect-only statements, and unresolved names.
// Allowed is true exactly when Status is CheckAllowed.
type CommandCheck struct {
	Command string       `json:"command"`
	Line    uint         `json:"line"`
	Column  uint         `json:"column"`
	Allowed bool         `json:"allowed"`
	Status  CheckStatus  `json:"status"`
	Issues  []CheckIssue `json:"issues,omitempty"`
}

// CheckResult is an advisory, non-executing policy report. Allowed means all
// command sites could be checked and passed; denied takes precedence over
// indeterminate. Issues contains script-wide errors; per-command explanations
// are in Commands. Warnings contains configuration diagnostics.
type CheckResult struct {
	Allowed  bool           `json:"allowed"`
	Status   CheckStatus    `json:"status"`
	Commands []CommandCheck `json:"commands"`
	Issues   []CheckIssue   `json:"issues,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
}

// MaxCheckCommands bounds the number of command sites in one Check report.
const MaxCheckCommands = 16 << 10

const maxCheckDepth = 128

// Check parses script and checks its statically determinable command, path,
// mode, elevation, and system-service permissions using this Runner's current
// state and configuration. It never executes a builtin, evaluates a command
// substitution, consumes stdin, opens a redirect, or invokes elevation. It may
// read filesystem metadata through the sandbox to check path containment.
// Neither the runner's variables nor its working directory are changed.
//
// Every syntactic branch is checked once, including branches that might not
// execute. Simple assignments and configured environment values can be
// resolved; data-dependent expansion, globbing, directory changes, and
// content-dependent builtin operations produce CheckIndeterminate rather than
// an approval. Denials and parse errors are returned in the report; the error
// return is reserved for cancellation, timeouts, and an invalid Runner.
//
// CheckAllowed means authorization, not successful execution: this does not
// predict exit codes, validate embedded programs, verify file existence or OS
// privileges, or guarantee termination. Policies must still be enforced by
// Run; filesystem state may change after Check. Like Run, Check is not safe to
// call concurrently with other methods on the same Runner.
func (r *Runner) Check(ctx context.Context, script string) (*CheckResult, error) {
	if r == nil || !r.usedNew {
		return nil, fmt.Errorf("use interp.New to construct a Runner")
	}
	if r.maxExecutionTime > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.maxExecutionTime)
		defer cancel()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Initialize a private state if Run has never initialized the runner.
	// Reset only installs handlers and shell variables; none is invoked here.
	copyRunner := *r
	if !copyRunner.didReset {
		copyRunner.stdout, copyRunner.stderr = io.Discard, io.Discard
		copyRunner.Reset()
	}
	state := &checkState{env: copyRunner.writeEnv, values: map[string]checkValue{}, dir: r.Dir, dirKnown: true, budget: &checkBudget{ctx: ctx}}
	policy := checkPolicy{
		allowedCommands: r.allowedCommands, allowAllCommands: r.allowAllCommands,
		elevatableCommands: r.elevatableCommands, elevationEnabled: r.elevate != nil,
		remediationMode: r.remediationMode, systemServices: r.allowedSystemServices,
		paths: r.sandbox, pathsConfigured: r.sandbox != nil,
	}
	return checkScriptPolicy(ctx, script, policy, state, r.Warnings())
}

// checkPathPolicy exposes only authorization, never executable file handles.
// Sandbox implements it for local checks; LexicalPolicy for remote checks.
type checkPathPolicy interface {
	CheckPath(path, cwd string, operation allowedpaths.PathOperation) error
	PathAccesses() []allowedpaths.PathAccess
}

type checkPolicy struct {
	allowedCommands    map[string]bool
	allowAllCommands   bool
	elevatableCommands map[string]bool
	elevationEnabled   bool
	remediationMode    bool
	systemServices     systemdGrants
	paths              checkPathPolicy
	pathsConfigured    bool
	remote             bool
}

func checkScriptPolicy(ctx context.Context, script string, policy checkPolicy, state *checkState, warnings []string) (*CheckResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := &CheckResult{Status: CheckAllowed, Commands: []CommandCheck{}, Warnings: warnings}
	program, err := ParseScript(script, "")
	if err != nil {
		result.Status = CheckDenied
		result.Issues = []CheckIssue{{Code: CheckParseError, Message: err.Error()}}
		return result, ctx.Err()
	}
	if err := validateNode(program, policy.remediationMode); err != nil {
		result.Status = CheckDenied
		result.Issues = []CheckIssue{{Code: CheckUnsupportedSyntax, Message: err.Error()}}
		return result, ctx.Err()
	}
	checker := scriptChecker{policy: policy, ctx: ctx, result: result}
	checker.stmts(program.Stmts, state, 0)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for i := range result.Commands {
		command := &result.Commands[i]
		command.Allowed = command.Status == CheckAllowed
		result.Status = combineCheckStatus(result.Status, command.Status)
	}
	result.Allowed = result.Status == CheckAllowed
	return result, nil
}

func combineCheckStatus(a, b CheckStatus) CheckStatus {
	if a == CheckDenied || b == CheckDenied {
		return CheckDenied
	}
	if a == CheckIndeterminate || b == CheckIndeterminate {
		return CheckIndeterminate
	}
	return CheckAllowed
}

func (c *CommandCheck) issue(code CheckIssueCode, message string) {
	status := CheckDenied
	if code == CheckRequiresExecution {
		status = CheckIndeterminate
	}
	c.Status = combineCheckStatus(c.Status, status)
	c.Issues = append(c.Issues, CheckIssue{Code: code, Message: message})
}

type checkValue struct {
	value string
	known bool
}

type checkState struct {
	env        expand.Environ
	values     map[string]checkValue
	valueBytes int
	envUnknown bool
	dir        string
	dirKnown   bool
	pipeline   bool
	budget     *checkBudget
}

type checkBudget struct {
	ctx   context.Context
	bytes int
}

func (s *checkState) clone() *checkState {
	copy := *s
	copy.values = make(map[string]checkValue, len(s.values))
	for name, value := range s.values {
		if s.budget.ctx.Err() != nil {
			break
		}
		copy.values[name] = value
	}
	return &copy
}

func (s *checkState) setValue(name string, value checkValue) {
	s.valueBytes += len(value.value) - len(s.values[name].value)
	s.values[name] = value
}

func (s *checkState) value(name string) checkValue {
	if name == "?" {
		return checkValue{}
	}
	if value, ok := s.values[name]; ok {
		return value
	}
	vr := s.env.Get(name)
	if !vr.Declared() && runtime.GOOS == "windows" {
		if value, ok := s.values[strings.ToUpper(name)]; ok {
			return value
		}
		vr = s.env.Get(strings.ToUpper(name))
	}
	return checkValue{value: vr.Str, known: !s.envUnknown || vr.ReadOnly}
}

func (s *checkState) Get(name string) expand.Variable {
	value := s.value(name)
	return expand.Variable{Set: true, Kind: expand.String, Str: value.value}
}

func (s *checkState) Each(fn func(string, expand.Variable) bool) {
	// Expansion only uses Get. Keep the environment interface complete without
	// exposing a partially known environment to an accidental enumeration.
}

// expandable permits only word expansion that requires no I/O. Quoted glob
// characters and escaped metacharacters remain literal. Brace expansions are
// left indeterminate, avoiding eager combinatorial allocation by the expander.
func (s *checkState) expandable(parts []syntax.WordPart, quoted bool) bool {
	for _, part := range parts {
		switch part := part.(type) {
		case *syntax.Lit:
			if quoted {
				continue
			}
			for i := 0; i < len(part.Value); i++ {
				if part.Value[i] == '\\' {
					i++
					continue
				}
				if strings.ContainsRune("*?[{", rune(part.Value[i])) {
					return false
				}
			}
		case *syntax.SglQuoted:
		case *syntax.DblQuoted:
			if !s.expandable(part.Parts, true) {
				return false
			}
		case *syntax.ParamExp:
			if part.Param == nil || !s.value(part.Param.Value).known || (!quoted && !s.value("IFS").known) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// boundWord accounts for expansion size before calling the expander, which may
// otherwise eagerly concatenate many copies of a large variable in one word.
// All branch/subshell copies share this budget.
func (s *checkState) boundWord(word *syntax.Word, quoted bool, maxBytes int) bool {
	if s.budget.ctx.Err() != nil || !s.expandable(word.Parts, quoted) {
		return false
	}
	remaining := maxBytes
	syntax.Walk(word, func(node syntax.Node) bool {
		if remaining < 0 || s.budget.ctx.Err() != nil {
			return false
		}
		switch node := node.(type) {
		case *syntax.Lit:
			remaining -= len(node.Value)
		case *syntax.SglQuoted:
			remaining -= len(node.Value)
		case *syntax.ParamExp:
			remaining -= len(s.value(node.Param.Value).value)
			return false
		}
		return true
	})
	if remaining < 0 || maxBytes-remaining > MaxExpandedBytesPerRun-s.budget.bytes {
		return false
	}
	s.budget.bytes += maxBytes - remaining
	return true
}

func (s *checkState) expansionConfig() *expand.Config {
	return &expand.Config{Env: s, ReadDir2: func(string) ([]fs.DirEntry, error) {
		return nil, fmt.Errorf("glob expansion requires execution")
	}}
}

func (s *checkState) literal(word *syntax.Word) (string, bool) {
	if word == nil {
		return "", true
	}
	if !s.boundWord(word, true, MaxVarBytes) {
		return "", false
	}
	value, err := expand.Literal(s.expansionConfig(), word)
	return value, err == nil && len(value) <= MaxVarBytes
}

type scriptChecker struct {
	policy  checkPolicy
	ctx     context.Context
	result  *CheckResult
	stopped bool
}

func (c *scriptChecker) limit() {
	if !c.stopped {
		c.result.Issues = append(c.result.Issues, CheckIssue{Code: CheckLimitExceeded, Message: "static check exceeds command or nesting limit"})
		c.result.Status = CheckIndeterminate
		c.stopped = true
	}
}

func (c *scriptChecker) add(pos syntax.Pos) *CommandCheck {
	if len(c.result.Commands) >= MaxCheckCommands {
		c.limit()
		return nil
	}
	// Commands are assembled locally by callers before append: pointers into
	// this slice must not survive recursive visits that may grow the slice.
	return &CommandCheck{Line: pos.Line(), Column: pos.Col(), Status: CheckAllowed}
}

func (c *scriptChecker) stmts(stmts []*syntax.Stmt, state *checkState, depth int) {
	for _, stmt := range stmts {
		c.stmt(stmt, state, depth)
	}
}

func (c *scriptChecker) stmt(stmt *syntax.Stmt, state *checkState, depth int) {
	if c.stopped || c.ctx.Err() != nil {
		return
	}
	if depth > maxCheckDepth {
		c.limit()
		return
	}
	if call, ok := stmt.Cmd.(*syntax.CallExpr); ok {
		c.call(stmt, call, state, depth)
		return
	}
	if len(stmt.Redirs) > 0 {
		entry := c.add(stmt.Pos())
		if entry == nil {
			return
		}
		c.redirects(entry, stmt.Redirs, state)
		c.result.Commands = append(c.result.Commands, *entry)
		for _, redirect := range stmt.Redirs {
			c.substitutions(redirect, state, depth)
		}
	}
	switch command := stmt.Cmd.(type) {
	case *syntax.Block:
		c.stmts(command.Stmts, state, depth+1)
	case *syntax.Subshell:
		c.stmts(command.Stmts, state.clone(), depth+1)
	case *syntax.BinaryCmd:
		if command.Op == syntax.Pipe {
			left, right := state.clone(), state.clone()
			left.pipeline, right.pipeline = true, true
			c.stmt(command.X, left, depth+1)
			c.stmt(command.Y, right, depth+1)
		} else {
			c.stmt(command.X, state, depth+1)
			c.stmt(command.Y, state.clone(), depth+1)
			c.invalidate(command.Y, state)
		}
	case *syntax.IfClause:
		c.invalidate(command, state)
		c.ifClause(command, state, depth+1)
	case *syntax.WhileClause:
		c.invalidate(command, state)
		c.stmts(command.Cond, state.clone(), depth+1)
		c.stmts(command.Do, state.clone(), depth+1)
	case *syntax.ForClause:
		c.invalidate(command, state)
		c.substitutions(command.Loop, state, depth)
		c.stmts(command.Do, state.clone(), depth+1)
	}
}

func (c *scriptChecker) ifClause(clause *syntax.IfClause, state *checkState, depth int) {
	if depth > maxCheckDepth {
		c.limit()
		return
	}
	c.stmts(clause.Cond, state.clone(), depth)
	c.stmts(clause.Then, state.clone(), depth)
	if clause.Else != nil {
		c.ifClause(clause.Else, state.clone(), depth+1)
	}
}

// invalidate forgets effects that depend on branches or loop iterations.
// Subshell and pipeline effects never change the surrounding environment.
func (c *scriptChecker) invalidate(node syntax.Node, state *checkState) {
	syntax.Walk(node, func(node syntax.Node) bool {
		if c.ctx.Err() != nil {
			return false
		}
		switch node := node.(type) {
		case *syntax.CmdSubst, *syntax.Subshell:
			return false
		case *syntax.BinaryCmd:
			return node.Op != syntax.Pipe
		case *syntax.WordIter:
			state.setValue(node.Name.Value, checkValue{})
		case *syntax.CallExpr:
			if len(node.Args) == 0 {
				for _, assign := range node.Assigns {
					if c.ctx.Err() != nil {
						return false
					}
					state.setValue(assign.Name.Value, checkValue{})
				}
			} else if name, known := state.literal(node.Args[0]); known {
				if _, registered := builtins.Lookup(name); !registered {
					// This includes sudo and expansions that split into several
					// fields: the actual state-changing command is not this word.
					name = ""
				}
				c.forgetCommandEffects(name, state)
			} else {
				c.forgetCommandEffects("", state)
			}
			return false
		}
		return true
	})
}

func (c *scriptChecker) forgetCommandEffects(name string, state *checkState) {
	if name == "read" || name == "" {
		state.envUnknown = true
		for variable := range state.values {
			if c.ctx.Err() != nil {
				return
			}
			state.setValue(variable, checkValue{})
		}
	}
	if name == "cd" || name == "" {
		state.dirKnown = false
		state.setValue("PWD", checkValue{})
		state.setValue("OLDPWD", checkValue{})
	}
}

func (c *scriptChecker) call(stmt *syntax.Stmt, call *syntax.CallExpr, state *checkState, depth int) {
	entry := c.add(stmt.Pos())
	if entry == nil {
		return
	}
	fields := []string{}
	complete := true
	bytes := 0
	for _, word := range call.Args {
		if !state.boundWord(word, false, MaxExpandedBytesPerCommand) {
			complete = false
			break
		}
		for field, err := range expand.FieldsSeq(state.expansionConfig(), word) {
			if err != nil {
				complete = false
				break
			}
			bytes += len(field)
			if len(fields) >= MaxExpandedArgumentsPerCommand+1 || bytes > MaxExpandedBytesPerCommand {
				entry.issue(CheckLimitExceeded, "command expansion exceeds argument or byte limit")
				complete = false
				break
			}
			fields = append(fields, field)
		}
		if !complete {
			break
		}
	}
	elevated := len(fields) > 0 && fields[0] == "sudo"
	if elevated {
		fields = fields[1:]
	}
	if len(fields) > 0 {
		entry.Command = fields[0]
		c.commandPolicy(entry, elevated, state.pipeline)
	} else if elevated && complete {
		entry.issue(CheckInvalidArguments, "sudo: command is required")
	}
	if !complete {
		entry.issue(CheckRequiresExecution, "command arguments depend on runtime expansion or globbing")
	}
	if entry.Status != CheckDenied {
		c.redirects(entry, stmt.Redirs, state)
		assignmentState := state
		if len(call.Assigns) > 0 {
			assignmentState = state.clone()
		}
		for _, assign := range call.Assigns {
			if c.ctx.Err() != nil {
				return
			}
			name := assign.Name.Value
			if state.env.Get(name).ReadOnly {
				entry.issue(CheckReadonlyVariable, name+": readonly variable")
				continue
			}
			value, known := assignmentState.literal(assign.Value)
			total := assignmentState.valueBytes - len(assignmentState.values[name].value) + len(value)
			if total > MaxTotalVarsBytes {
				entry.issue(CheckLimitExceeded, "assignments exceed variable storage limit")
				value, known = "", false
			}
			assignmentState.setValue(name, checkValue{value: value, known: known})
			if !known {
				entry.issue(CheckRequiresExecution, "assignment value requires execution")
			}
		}
		if complete && len(fields) > 0 {
			// argv and redirects expand against the original environment, but
			// the builtin sees inline assignments (notably HOME/OLDPWD for cd).
			c.arguments(entry, fields[1:], assignmentState)
		}
		if len(fields) == 0 && complete && entry.Status != CheckDenied {
			state.values = assignmentState.values
			state.valueBytes = assignmentState.valueBytes
		}
	}
	c.result.Commands = append(c.result.Commands, *entry)
	c.substitutions(call, state, depth)
	for _, redirect := range stmt.Redirs {
		c.substitutions(redirect, state, depth)
	}
	if len(call.Args) > 0 && entry.Status != CheckDenied {
		c.forgetCommandEffects(entry.Command, state)
	}
}

func (c *scriptChecker) commandPolicy(entry *CommandCheck, elevated, pipeline bool) {
	name := entry.Command
	if !c.policy.allowAllCommands && !c.policy.allowedCommands[name] {
		entry.issue(CheckCommandNotAllowed, name+": command not allowed")
	}
	if _, known := builtins.Lookup(name); !known {
		entry.issue(CheckUnknownCommand, name+": unknown command")
	}
	if elevated && (!c.policy.elevationEnabled || !c.policy.elevatableCommands[name]) {
		entry.issue(CheckElevationNotAllowed, name+": elevation not allowed")
	}
	if elevated && pipeline {
		entry.issue(CheckElevationNotAllowed, "elevated commands are not allowed in pipelines")
	}
	if message, denied := remediationOnlyRefusal(name, c.policy.remediationMode); denied {
		entry.issue(CheckRemediationRequired, message)
	}
}

func (c *scriptChecker) substitutions(node syntax.Node, state *checkState, depth int) {
	if depth > maxCheckDepth {
		c.limit()
		return
	}
	syntax.Walk(node, func(node syntax.Node) bool {
		if c.stopped || c.ctx.Err() != nil {
			return false
		}
		if substitution, ok := node.(*syntax.CmdSubst); ok {
			if len(substitution.Stmts) == 1 && catShortcutArg(substitution.Stmts[0]) != nil {
				entry := c.add(substitution.Pos())
				if entry != nil {
					entry.Command = "cat"
					c.commandPolicy(entry, false, state.pipeline)
					c.redirects(entry, substitution.Stmts[0].Redirs, state)
					c.result.Commands = append(c.result.Commands, *entry)
					for _, redirect := range substitution.Stmts[0].Redirs {
						c.substitutions(redirect, state, depth+1)
					}
				}
			} else {
				c.stmts(substitution.Stmts, state.clone(), depth+1)
			}
			return false
		}
		return true
	})
}

func (c *scriptChecker) redirects(entry *CommandCheck, redirects []*syntax.Redirect, state *checkState) {
	for _, redirect := range redirects {
		op := allowedpaths.PathRead
		switch redirect.Op {
		case syntax.RdrIn:
		case syntax.RdrOut, syntax.AppOut, syntax.ClbOut, syntax.RdrAll, syntax.AppAll:
			op = allowedpaths.PathWrite
		default:
			continue
		}
		known := state.boundWord(redirect.Word, false, MaxExpandedBytesPerCommand)
		var paths []string
		if known {
			for path, err := range expand.FieldsSeq(state.expansionConfig(), redirect.Word) {
				if err != nil || len(path) > MaxExpandedBytesPerCommand {
					known = false
					break
				}
				paths = append(paths, path)
				if len(paths) > 1 {
					break
				}
			}
		}
		if !known {
			entry.issue(CheckRequiresExecution, "redirect target requires execution")
			continue
		}
		if len(paths) != 1 {
			entry.issue(CheckInvalidArguments, "ambiguous redirect")
			continue
		}
		path := paths[0]
		// Only output redirects implement the unconditional null-device sink.
		if op == allowedpaths.PathWrite && isDevNull(path) {
			continue
		}
		c.path(entry, path, op, state)
	}
}

func (c *scriptChecker) path(entry *CommandCheck, path string, operation allowedpaths.PathOperation, state *checkState) {
	if (operation == allowedpaths.PathWrite || operation == allowedpaths.PathRemove) && !c.policy.remediationMode {
		entry.issue(CheckRemediationRequired, "file writes require remediation mode")
		return
	}
	if c.policy.remote {
		roots := c.policy.paths.PathAccesses()
		writable := false
		for _, root := range roots {
			writable = writable || root.ReadWrite
		}
		message := ""
		if len(roots) == 0 && !((operation == allowedpaths.PathStat || operation == allowedpaths.PathLstat) && allowedpaths.IsDevNull(path)) {
			message = "no path grants are configured"
		} else if (operation == allowedpaths.PathWrite || operation == allowedpaths.PathRemove) && !writable {
			message = "no writable path grants are configured"
		}
		if message != "" {
			entry.issue(CheckPathNotAllowed, message)
			entry.Issues[len(entry.Issues)-1].Path = path
			return
		}
	}
	if !state.dirKnown && !filepath.IsAbs(path) {
		message := "relative path depends on a runtime working directory"
		if c.policy.remote {
			message = "relative path requires an explicit remote working directory or execution on the target"
		}
		entry.issue(CheckRequiresExecution, message)
		entry.Issues[len(entry.Issues)-1].Path = path
		return
	}
	if err := c.policy.paths.CheckPath(path, state.dir, operation); err != nil {
		entry.issue(CheckPathNotAllowed, err.Error())
		entry.Issues[len(entry.Issues)-1].Path = path
	} else if c.policy.remote {
		entry.issue(CheckRequiresExecution, "remote filesystem must verify grant roots, symlink containment, file type, hard-link restrictions, and OS access")
		entry.Issues[len(entry.Issues)-1].Path = path
	}
}
