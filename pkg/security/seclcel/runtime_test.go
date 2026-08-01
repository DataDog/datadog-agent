// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package seclcel

import (
	"fmt"
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// evalSECL translates a SECL expression, plans it, and evaluates it against an
// event.
func evalSECL(t *testing.T, env *cel.Env, event *model.Event, expr string) bool {
	t.Helper()

	program, err := Program(env, expr, ModelFieldTypes{})
	require.NoError(t, err, "planning %q", expr)

	out, _, err := program.Eval(NewActivation(eval.NewContext(event)))
	require.NoError(t, err, "evaluating %q", expr)

	result, ok := out.(types.Bool)
	require.True(t, ok, "%q returned %s rather than a boolean", expr, out.Type())
	return bool(result)
}

// ancestry builds a process ancestor chain, nearest ancestor first.
func ancestry(event *model.Event, comms []string, uids []uint32) {
	var head *model.ProcessCacheEntry
	for i := len(comms) - 1; i >= 0; i-- {
		entry := &model.ProcessCacheEntry{
			ProcessContext: model.ProcessContext{
				Process: model.Process{
					Comm:        comms[i],
					Credentials: model.Credentials{UID: uids[i]},
				},
				Ancestor: head,
			},
		}
		head = entry
	}
	event.BaseEvent.ProcessContext.Ancestor = head
}

func TestProgramScalarFields(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	event := model.NewFakeEvent()
	event.BaseEvent.ProcessContext.Process.Comm = "sh"
	event.BaseEvent.ProcessContext.Process.Credentials.UID = 1000
	event.BaseEvent.ProcessContext.Process.FileEvent.BasenameStr = "bash"
	event.BaseEvent.ProcessContext.Process.FileEvent.PathnameStr = "/usr/bin/bash"

	tests := []struct {
		expr string
		want bool
	}{
		{`process.comm == "sh"`, true},
		{`process.comm == "zsh"`, false},
		{`process.uid == 1000`, true},
		{`process.uid > 1000`, false},
		{`process.comm in [ "sh", "bash" ]`, true},
		{`process.comm not in [ "sh" ]`, false},

		// the glob helper, reached through =~ and through a pattern literal
		{`process.file.path =~ "/usr/bin/*"`, true},
		{`process.file.path =~ "/etc/*"`, false},
		{`process.file.path == ~"/usr/*/bash"`, true},

		// the regexp path, which is CEL's own matches()
		{`process.file.name == r"^ba.*"`, true},
		{`process.file.name == r"^zs.*"`, false},

		// size() over a scalar string
		{`process.comm.length == 2`, true},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			assert.Equal(t, tt.want, evalSECL(t, env, event, tt.expr))
		})
	}
}

func TestProgramAncestors(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	event := model.NewFakeEvent()
	ancestry(event,
		[]string{"bash", "sshd", "init"},
		[]uint32{1000, 0, 0})

	tests := []struct {
		expr string
		want bool
	}{
		// a comparison against an iterated field asks whether some element matches
		{`process.ancestors.comm == "sshd"`, true},
		{`process.ancestors.comm == "zsh"`, false},
		{`process.ancestors.comm in [ "init" ]`, true},
		{`process.ancestors.comm not in [ "bash", "sshd", "init" ]`, false},

		// allin asks whether every element matches
		{`process.ancestors.uid allin [ 0 ]`, false},
		{`process.ancestors.comm allin [ "bash", "sshd", "init" ]`, true},

		// an iterator variable correlates the two comparisons at one index
		{`process.ancestors[A].comm == "bash" && process.ancestors[A].uid == 1000`, true},
		// the negative control: both hold, but never at the same index
		{`process.ancestors[A].comm == "bash" && process.ancestors[A].uid == 0`, false},
		{`process.ancestors[A].comm == "sshd" && process.ancestors[A].uid == 0`, true},

		// the element count
		{`process.ancestors.length == 3`, true},

		// a constant subscript picks one element out
		{`process.ancestors[0].comm == "bash"`, true},
		{`process.ancestors[2].comm == "init"`, true},
		{`process.ancestors[1].comm == "init"`, false},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			assert.Equal(t, tt.want, evalSECL(t, env, event, tt.expr))
		})
	}
}

// TestProgramRegisterCacheInvalidation guards the subtlety that the accessors
// cache a resolved element per register, so the cache has to be dropped when the
// index moves. Without it every ancestor would read as the first one.
func TestProgramRegisterCacheInvalidation(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	event := model.NewFakeEvent()
	comms := []string{"first", "second", "third"}
	ancestry(event, comms, []uint32{1, 2, 3})

	for i, comm := range comms {
		expr := fmt.Sprintf(`process.ancestors[A].comm == %q && process.ancestors[A].uid == %d`, comm, i+1)
		assert.Equal(t, true, evalSECL(t, env, event, expr), expr)
	}
}

// TestProgramIterationIsNotCapped documents a known divergence rather than a
// guarantee. SECL bounds iteration at maxRegisterIteration, but only for a rule
// written with an explicit iterator variable; an implicit comparison against an
// array field is unbounded. Both translate to the same exists(), so the runtime
// cannot distinguish them and applies no bound, matching the common form.
func TestProgramIterationIsNotCapped(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	comms := make([]string, 150)
	uids := make([]uint32, 150)
	for i := range comms {
		comms[i] = fmt.Sprintf("proc%d", i)
	}

	event := model.NewFakeEvent()
	ancestry(event, comms, uids)

	assert.True(t, evalSECL(t, env, event, `process.ancestors.comm == "proc50"`))

	// SECL agrees here, because an implicit array comparison is unbounded.
	assert.True(t, evalSECL(t, env, event, `process.ancestors.comm == "proc120"`),
		"an implicit array comparison sees every element in both engines")

	// The explicit form is where the two differ: SECL would stop at 100.
	assert.True(t, evalSECL(t, env, event, `process.ancestors[A].comm == "proc120"`),
		"the register form is unbounded here but bounded in SECL")

	assert.True(t, evalSECL(t, env, event, `process.ancestors.length == 150`))
}

// TestProgramDuration checks the secl_now form end to end. The fake field
// handlers stub process.created_at to zero, so the field is degenerate — but the
// path under test is the same: secl_now resolves from the context, the
// subtraction runs, and secl.nanos turns the result into a duration.
func TestProgramDuration(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	event := model.NewFakeEvent()
	ctx := eval.NewContext(event)

	// the activation supplies the instant the evaluation started
	now, iss := env.Compile(NowVar + " > 0")
	require.NoError(t, iss.Err())
	program, err := env.Program(now)
	require.NoError(t, err)
	out, _, err := program.Eval(NewActivation(ctx))
	require.NoError(t, err)
	assert.Equal(t, types.True, out, "secl_now must resolve from the context")

	// created_at is zero, so the elapsed time since it is the whole epoch
	assert.False(t, evalSECL(t, env, event, `process.created_at < 10m`))
	assert.True(t, evalSECL(t, env, event, `process.created_at > 10m`))

	// the arithmetic form is an interval already and needs no instant
	assert.True(t, evalSECL(t, env, event, `process.created_at - process.created_at < 10m`))
}

func TestHelperBindings(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	event := model.NewFakeEvent()
	event.BaseEvent.ProcessContext.Process.Comm = "sh"

	tests := []struct {
		expr string
		want bool
	}{
		// glob, including SECL's whole-string anchoring
		{`"/usr/bin/ls" == ~"/usr/*"`, true},
		{`"/usr/bin/ls" == ~"/usr/*/ls"`, true},
		{`"/usr/bin/ls" == ~"/usr"`, false},

		// the CIDR overlap rule: either range containing the other's base address
		{`10.0.0.1 in 10.0.0.0/8`, true},
		{`10.0.0.1 in 192.168.0.0/16`, false},
		{`10.0.0.0/8 in 10.1.0.0/16`, true},
		{`10.0.0.1 in [ 192.168.0.0/16, 10.0.0.0/8 ]`, true},
		{`10.0.0.1 allin [ 192.168.0.0/16, 10.0.0.0/8 ]`, false},
		{`10.0.0.1 allin [ 10.0.0.0/8, 10.0.0.0/24 ]`, true},

		// string interpolation of a field reference, through the str helper
		{`"proc-%{process.comm}" == "proc-sh"`, true},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			assert.Equal(t, tt.want, evalSECL(t, env, event, tt.expr))
		})
	}
}

// TestProgramResolvesOnlyWhatIsMentioned is the claim the design rests on: a field
// is read when a leaf is reached, so nothing is materialised and short circuiting
// avoids the work entirely.
func TestProgramResolvesOnlyWhatIsMentioned(t *testing.T) {
	reads := map[string]int{}

	env := countingEnv(t, reads)

	event := model.NewFakeEvent()
	event.BaseEvent.ProcessContext.Process.Comm = "sh"
	ancestry(event, []string{"bash", "sshd"}, []uint32{1000, 0})

	// The right hand side is never reached, so no ancestor is touched.
	assert.Equal(t, true, evalSECL(t, env, event,
		`process.comm == "sh" || process.ancestors.comm == "zzz"`))

	assert.Equal(t, 1, reads["comm"], "the mentioned field is read once")
	assert.Zero(t, reads["ancestors"], "the short circuited side is not touched")

	// The elements of an iterated field are positions rather than values, so
	// building the list reads nothing and a matching predicate stops early.
	clear(reads)
	assert.True(t, evalSECL(t, env, event, `process.ancestors.comm == "bash"`))
	assert.Equal(t, 1, reads["comm"], "the match is at the first element, so only it is read")

	clear(reads)
	assert.False(t, evalSECL(t, env, event, `process.ancestors.comm == "zzz"`))
	assert.Equal(t, 2, reads["comm"], "no match, so every element is read once")
}

// TestActivationOutlivesTheEvent pins what the activation's root cache rests
// on: the values it hands out hold the context rather than the event, and every
// read goes through ctx.Event, so an activation stays correct after its context
// has been reset onto the next event.
//
// Without that, caching a root would pin the event it was first resolved
// against, and a rule would go on matching the event before it.
func TestActivationOutlivesTheEvent(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	program, err := Program(env, `process.comm == "sh"`, ModelFieldTypes{})
	require.NoError(t, err)

	matching := model.NewFakeEvent()
	matching.BaseEvent.ProcessContext.Process.Comm = "sh"

	other := model.NewFakeEvent()
	other.BaseEvent.ProcessContext.Process.Comm = "zsh"

	ctx := eval.NewContext(matching)
	activation := NewActivation(ctx)

	matches := func() bool {
		t.Helper()
		out, _, err := program.Eval(activation)
		require.NoError(t, err)
		return out == types.True
	}

	assert.True(t, matches(), "the event the activation was built on")

	// The rule engine reuses a context across events, so the same activation
	// sees the next one.
	ctx.Reset()
	ctx.SetEvent(other)
	assert.False(t, matches(), "the next event, through the same activation")

	ctx.Reset()
	ctx.SetEvent(matching)
	assert.True(t, matches(), "and back again")
}

// TestPartialEvaluation records that cel-go's partial evaluation reaches through
// the custom activation and type provider, which matters twice over.
//
// It is the mechanism SECL's Rule.PartialEval provides and that CWS depends on
// in three places — approvers (rules/approvers.go:60), discarders
// (probe/discarders_linux.go:585) and ruleset.go:1045 — to decide whether a rule
// could still match if only one field were known, and so what can be filtered in
// eBPF. Nothing about the node graph had to change for it to work.
//
// It also guards the activation's root cache: a cached root that resolved past
// the unknown patterns would silently turn an unknown into a value, and the
// approvers built from it would be wrong.
func TestPartialEvaluation(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	checked, err := CompileWithTypes(env, `process.comm == "sh" && process.uid == 1000`, ModelFieldTypes{})
	require.NoError(t, err)

	program, err := env.Program(checked, cel.EvalOptions(cel.OptPartialEval))
	require.NoError(t, err)

	event := model.NewFakeEvent()
	event.BaseEvent.ProcessContext.Process.Comm = "sh"
	event.BaseEvent.ProcessContext.Process.Credentials.UID = 999

	// process.uid is withheld, so the answer depends on what it turns out to be.
	partial, err := cel.PartialVars(NewActivation(eval.NewContext(event)),
		cel.AttributePattern("process").QualString("uid"))
	require.NoError(t, err)

	out, _, err := program.Eval(partial)
	require.NoError(t, err)
	require.True(t, types.IsUnknown(out), "got %s", out)
	assert.Contains(t, out.(*types.Unknown).String(), "process.uid",
		"the unknown names the field the outcome hangs on")

	// The half that is known still decides it when it can, which is what makes
	// the answer useful rather than always unknown.
	event.BaseEvent.ProcessContext.Process.Comm = "zsh"
	out, _, err = program.Eval(partial)
	require.NoError(t, err)
	assert.Equal(t, types.False, out, "no value of process.uid could make this match")
}

// TestProgramWalksAnAncestryOnce is the regression test for the cost the node
// graph was built to remove.
//
// An iterated field used to be presented as a list of positions, which meant
// asking how many there were — a full walk of the ancestry — before the
// predicate saw the first element, and then resolving each position by walking
// from the root again. Reading positions 0, 1, 2… therefore cost O(N²) on a
// chain nothing had asked to see all of.
//
// A cursor yields the elements instead, so the chain is walked once and only as
// far as the predicate needs.
func TestProgramWalksAnAncestryOnce(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	comms := make([]string, 150)
	uids := make([]uint32, 150)
	for i := range comms {
		comms[i] = fmt.Sprintf("proc%d", i)
		uids[i] = uint32(i)
	}

	event := model.NewFakeEvent()
	ancestry(event, comms, uids)

	// One element of lookahead is inherent: a CEL fold asks whether there is a
	// next element before it checks whether it still wants one.
	const lookahead = 1

	tests := []struct {
		expr    string
		matches bool
		steps   int
	}{
		// The match is at the first element, so the walk stops there rather than
		// running the length of the chain.
		{`process.ancestors.comm == "proc0"`, true, 1 + lookahead},
		// No match, so the whole chain is walked — once, and the trailing step is
		// the one that reports it exhausted.
		{`process.ancestors.comm == "zzz"`, false, len(comms) + 1},
		// Two fields of one element read from one element, so correlating them at
		// the last one costs one walk rather than one per position before it.
		{`process.ancestors[A].comm == "proc149" && process.ancestors[A].uid == 149`, true, len(comms) + lookahead},
		// The count is the one thing that has to see every element.
		{`process.ancestors.length == 150`, true, len(comms) + 1},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			steps := countAncestorSteps(t)
			assert.Equal(t, tt.matches, evalSECL(t, env, event, tt.expr))
			assert.Equal(t, tt.steps, *steps, "elements walked by %q", tt.expr)
		})
	}
}

// countAncestorSteps replaces the ancestors cursor with one that counts the
// elements it yields, for the duration of the test.
//
// The cursor is bound to the field when a rule is planned, so this has to be in
// place before the program is built — which it is, because evalSECL plans afresh
// on every call.
func countAncestorSteps(t *testing.T) *int {
	t.Helper()

	var steps int

	const field = "process.ancestors"
	ancestors := celIterators[field]
	celIterators[field] = func(ctx *eval.Context) celCursor {
		return &countingCursor{inner: ancestors(ctx), steps: &steps}
	}
	t.Cleanup(func() { celIterators[field] = ancestors })

	return &steps
}

type countingCursor struct {
	inner celCursor
	steps *int
}

func (c *countingCursor) next() any {
	*c.steps++
	return c.inner.next()
}

// countingEnv is the model environment with the type provider wrapped so that
// every field read is counted.
func countingEnv(t *testing.T, reads map[string]int) *cel.Env {
	t.Helper()

	registry, err := types.NewRegistry()
	require.NoError(t, err)

	opts := []cel.EnvOption{cel.CustomTypeProvider(&countingProvider{
		Provider: &modelTypes{Provider: registry},
		reads:    reads,
	})}
	for name, root := range modelRoots {
		opts = append(opts, cel.Variable(name, root))
	}

	env, err := NewEnv(opts...)
	require.NoError(t, err)
	return env
}

type countingProvider struct {
	types.Provider
	reads map[string]int
}

func (c *countingProvider) FindStructFieldType(structType, field string) (*types.FieldType, bool) {
	fieldType, ok := c.Provider.FindStructFieldType(structType, field)
	if !ok {
		return fieldType, ok
	}

	getFrom := fieldType.GetFrom
	return &types.FieldType{
		Type:  fieldType.Type,
		IsSet: fieldType.IsSet,
		GetFrom: func(obj any) (any, error) {
			c.reads[field]++
			return getFrom(obj)
		},
	}, true
}

// TestProgramVariablesAreNotWired pins the phase 3 gap: SECL's ${…} variables are
// declared but not populated, so an expression using one fails rather than
// silently reading nothing.
func TestProgramVariablesAreNotWired(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	program, err := Program(env, `"proc-${my.var}" == "proc-sh"`, ModelFieldTypes{})
	require.NoError(t, err, "it translates and type-checks")

	_, _, err = program.Eval(NewActivation(eval.NewContext(model.NewFakeEvent())))
	require.Error(t, err, "but cannot be evaluated yet")
	assert.Contains(t, err.Error(), VariablesRoot)
}
