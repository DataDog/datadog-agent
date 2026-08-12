// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package seclcel

// These compare the cost of evaluating the same rule through the SECL evaluator
// and through the translated CEL program, which is the only way to tell where the
// translation actually costs something.
//
// What a figure below contains, beyond evaluating the rule:
//
//   - 30 ns of resetting the context, which both engines pay here. The rule
//     engine resets once per event and then evaluates the whole rule set, so
//     production amortises it over every rule where this charges it to each one.
//     It is in the loop regardless, because the SECL accessors memoise a
//     resolved array on the context: without a reset, an iterated field is
//     walked on the first iteration and never again, and the benchmark measures
//     a cache instead of a field. Subtract it to compare the engines on
//     evaluation alone — it is a larger share of SECL's figures, so leaving it
//     in flatters CEL slightly.
//   - Nothing else measurable. Checking the result costs 2.5 ns and is the same
//     either way.
//
// What is left out is building the CEL activation: 123 ns and 56 B in two
// allocations, once — the activation and the root position it hands out. That is
// right only if an activation lives as long as the pooled context it is bound to,
// which it may — see the note on the activation type. If the integration ends up
// building one per event, that cost belongs in these figures.
//
// As measured in one session:
//
//	                 SECL                 CEL
//	Comm             170 ns,  2 allocs    196 ns,   1
//	Path             186 ns,  2           198 ns,   1
//	InList           174 ns,  2           218 ns,   1
//	Regex            361 ns,  2           480 ns,   3
//	Glob             268 ns,  2           287 ns,   2
//	Ancestors       7.42 µs, 12          52.4 µs, 304
//	DeepAncestors   56.0 µs, 304         37.0 µs, 205
//
// Two of those are worth reading twice. Regex is where CEL is still clearly dearer,
// and the gap is cel-go's own dispatch rather than anything about fields.
// DeepAncestors is now cheaper through CEL than through SECL, which is the cursor
// paying off: SECL reaches each position by walking from the root again.
//
// Where the CEL column came from, in order: 672 ns for Comm when a field was
// resolved by joining its path and looking the accessor up by name; 250 ns once the
// readers were generated; 262 ns when the namespace was rooted under one variable;
// and 196 ns now that a field is read by index. The three shapes with a literal in
// them — InList, Regex, Glob — would cost 724, 2847 and 736 ns without
// planningOptions, since the list, the regexp and the glob pattern would be rebuilt
// on every event.
//
// What a scalar read is made of, the 196 ns of Comm: 30 ns is resetting the context,
// which SECL pays too; 47 ns is cel-go evaluating a program that reads nothing at
// all; 56 ns is boxing the string the field holds into a CEL value; and the rest is
// the comparison, the read and what surrounds it — see BenchmarkRead, which takes
// those apart, and BenchmarkCELScaffolding for what one Eval costs before any of it.
//
// A CPU profile puts the rest of it where nothing here can reach: acquiring and
// releasing the pooled ExecutionFrame is around a fifth of a scalar evaluation.
// cel-go will accept a frame passed to Eval instead of taking one from the pool,
// which is what BenchmarkCELScaffolding measures — that and the rest of what
// surrounds an evaluation is nearly two fifths of a bucket rule, and a larger share
// than it used to be now that the read itself is cheap. Its documentation says a
// frame must not be stored, so that remains a question for whoever wires up the
// rule engine rather than a change to make here.
//
// The same profile is what found the type adapter: every read returns a ref.Val
// already, and the interpreter adapted it anyway, through a type switch dozens of
// cases long, to produce the same value. seclAdapter was that one case, worth 6% of
// a scalar read then. It is no longer on that path — a read hands its value straight
// to the operator above it — so what it covers now is the rest of the library.
//
// Of an iterated evaluation, the fold itself — not the predicate — is most of
// the cost. Profiling foldIterable: 41% the step (accu || predicate), 28% the
// condition (!accu, which dispatches through runtime overload matching on every
// element), and 22% the cursor and the element value. The condition is pure
// bookkeeping: over a quarter of the work is the comprehension asking itself
// whether to continue.
//
// Path costs the same as Comm now, where it used to cost more: a field three
// segments deep and one two segments deep are both one read.
//
// The iterated cases are what the cursors were built for. Presenting an
// iterated field as a list of positions meant reading its length — a full walk
// of the ancestry — before the predicate saw the first element, and then
// resolving each position by walking from the root again, so reading positions
// 0, 1, 2… cost O(N²). A cursor yields the elements, so a match stops at the
// element that matched rather than paying for all of them first.
//
// Where the two engines now stand:
//
//   - A scalar rule is around 25 ns dearer through CEL, and BenchmarkScale says
//     that is a fixed cost per rule rather than per predicate: from two predicates
//     on, CEL is the cheaper of the two. What is left of the gap is what surrounds
//     one Eval, not what a field costs.
//   - A shallow match on a deep iterated field is dearer through SECL, because
//     an array field is resolved whole — 150 strings, 9 kB — before anything
//     compares them, where a cursor stops at the element that matched. No
//     benchmark shows this any more: since 71aa80a both ancestor shapes match at
//     the end of the chain, so the Ancestors row above measures a full walk
//     through both engines.
//   - A correlated read at depth is O(N) through CEL and O(N²) through SECL,
//     but the benchmark barely shows it. SECL clears the register cache after
//     every position (rule.go:246), so each position is reached by walking from
//     the root again — Σ(i+1) links — while a cursor makes one pass and reads
//     both fields off the element it yielded. At the sizes SECL allows the
//     quadratic term is not the bill: maxRegisterIteration caps the loop at 100
//     positions, so it tops out around 5000 links, and what dominates both
//     engines is the per-position work — SECL re-evaluating the whole rule,
//     allocating a one element slice per field read, and CEL running the
//     predicate once per element.
//
// The one structural advantage left to SECL is the per-event memoisation
// BenchmarkSECLWarmCache measures: a second rule mentioning an iterated field
// gets it for free within the same event, where CEL walks it again.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/interpreter"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

var benchExprs = map[string]string{
	// a direct struct read, so the dispatch dominates
	"Comm": `process.comm == "sh"`,
	// resolved through a field handler, which does real work
	"Path": `process.file.path == "/usr/bin/bash"`,
	// the shapes whose literals are worth compiling once, when the rule is
	// planned, rather than on every event
	"InList": `process.comm in [ "zsh", "bash", "sh" ]`,
	"Regex":  `process.file.name == r"^ba.*"`,
	"Glob":   `process.file.path =~ "/usr/bin/*"`,
	// four segments plus an iterated field
	"Ancestors": `process.ancestors.comm == "sshd"`,
	// the same, on an ancestry deep enough for the cost of reaching an element to
	// show, and correlating two fields of one element so that reaching it twice
	// would show too
	"DeepAncestors": fmt.Sprintf(`process.ancestors[A].comm == "proc%d" && process.ancestors[A].uid == %d`,
		benchDeepMatch, benchDeepMatch),
}

const (
	// benchAncestry is the depth of the ancestor chain the benchmarks run against.
	benchAncestry = 150
	// benchDeepMatch is where the deep benchmark matches. It has to stay under
	// SECL's maxRegisterIteration, which bounds the register form at 100
	// elements, or the two engines would not be measuring the same rule.
	benchDeepMatch = 99
)

func benchEvent() *model.Event {
	event := model.NewFakeEvent()
	event.BaseEvent.ProcessContext.Process.FileEvent.PathnameStr = "/usr/bin/bash"
	event.BaseEvent.ProcessContext.Process.FileEvent.BasenameStr = "bash"
	event.BaseEvent.ProcessContext.Process.Comm = "sh"

	// "sshd" is the last ancestor, so both ancestor benchmarks run to the end of
	// the chain. It was the second one until 71aa80a moved it, which is why the
	// Ancestors figures recorded before that commit are an order of magnitude
	// lower than the ones above.
	comms := make([]string, benchAncestry)
	uids := make([]uint32, benchAncestry)
	for i := range comms {
		comms[i] = fmt.Sprintf("proc%d", i)
		uids[i] = uint32(i)
	}
	comms[benchAncestry-1] = "sshd"

	ancestry(event, comms, uids)
	return event
}

func BenchmarkSECL(b *testing.B) {
	for name, expr := range benchExprs {
		b.Run(name, func(b *testing.B) {
			var m model.Model
			rule, err := eval.NewRule("bench", expr, ast.NewParsingContext(false), &eval.Opts{})
			if err != nil {
				b.Fatal(err)
			}
			if err := rule.GenEvaluator(&m); err != nil {
				b.Fatal(err)
			}

			event := benchEvent()
			ctx := eval.NewContext(event)
			b.ReportAllocs()
			for b.Loop() {
				resetContext(ctx, event)
				if !rule.Eval(ctx) {
					b.Fatal("expected a match")
				}
			}
		})
	}
}

// resetContext puts the context back into the state the rule engine hands one
// in, which it does once per event through eval.ContextPool.
//
// Without it a benchmark measures nothing real for an iterated field: the SECL
// accessors memoise a resolved array on the context, so reusing one context
// across iterations means the ancestry is walked on the first iteration and
// never again. Resetting costs about 30 ns, which is not what the difference is
// made of.
func resetContext(ctx *eval.Context, event *model.Event) {
	ctx.Reset()
	ctx.SetEvent(event)
}

// BenchmarkSECLWarmCache measures what that memoisation buys, which is the one
// place SECL is structurally ahead: within a single event, the second rule to
// mention an iterated field gets it for free, where CEL walks it again.
//
// It is a per-event cache, so it says nothing about the cost of the first read —
// which is what BenchmarkSECL measures.
func BenchmarkSECLWarmCache(b *testing.B) {
	var m model.Model
	rule, err := eval.NewRule("bench", benchExprs["Ancestors"], ast.NewParsingContext(false), &eval.Opts{})
	if err != nil {
		b.Fatal(err)
	}
	if err := rule.GenEvaluator(&m); err != nil {
		b.Fatal(err)
	}

	ctx := eval.NewContext(benchEvent())
	b.ReportAllocs()
	for b.Loop() {
		if !rule.Eval(ctx) {
			b.Fatal("expected a match")
		}
	}
}

func BenchmarkCEL(b *testing.B) {
	env, err := NewModelEnv()
	if err != nil {
		b.Fatal(err)
	}

	for name, expr := range benchExprs {
		b.Run(name, func(b *testing.B) {
			program, err := Program(env, expr, ModelFieldTypes{})
			if err != nil {
				b.Fatal(err)
			}

			event := benchEvent()
			ctx := eval.NewContext(event)
			activation := NewActivation(ctx)
			b.ReportAllocs()
			for b.Loop() {
				resetContext(ctx, event)
				out, _, err := program.Eval(activation)
				if err != nil {
					b.Fatal(err)
				}
				if out.Value() != true {
					b.Fatal("expected a match")
				}
			}
		})
	}
}

// BenchmarkCELScaffolding measures what surrounds an evaluation rather than the
// evaluation itself, by taking away, one at a time, everything cel-go's own
// BenchmarkInterpreter does without.
//
// Frame is the first: it builds an ExecutionFrame once and reuses it for every
// iteration, where BenchmarkCEL hands Eval an activation and lets it take a
// frame from the pool and return it. prog.Eval type switches on a frame and
// skips both that and the deferred Close, which a profile put at around a fifth
// of a scalar evaluation.
//
// The frame's documentation says it must not be stored, so what this measures
// is the size of the prize, not a licence to take it.
//
// The other half of cel-go's benchmark, CompileRegexConstants with
// MatchesRegexOptimization, has no variant here because we already have it:
// cel.OptOptimize registers that optimization itself (cel/program.go:246), so
// planningOptions gets it through the option it already passes. Measured as a
// variant it moved nothing, which is the confirmation.
//
// Exec is the rest of the same question. cel.Program is a wrapper around the
// interpretable the planner produced, and prog.Eval charges for the wrapper on
// every call: a deferred recover, the frame handling above, a types.IsError test
// and a three value return. cel-go's own benchmark never pays it — it plans
// through interpreter.NewInterpretable and calls Exec on the interpretable
// directly. planInterpretable rebuilds what newProgram builds so that the same
// rule can be evaluated that way here.
//
// Bucket/ is the same question asked where it matters most: both costs are per
// Eval, so a bucket of r rules pays them r times per event.
//
// Measured (ns/op, median of three runs at -cpu=1; allocations are identical
// across all three, the pool was already giving the frame back for free):
//
//	                 activation      frame       exec
//	Comm                    216        180        162
//	Path                    218        182        164
//	InList                  255        197        186
//	Regex                   583        529        512
//	Glob                    340        298        282
//	Ancestors             64056      64141      64579
//	DeepAncestors         44648      44636      44937
//	Bucket/10  per rule     198        136        113
//	Bucket/50  per rule     189        139        111
//	Bucket/200 per rule     215        153        132
//
// Both are costs per Eval rather than per predicate: 36 to 58 ns for the frame, and
// a further 10 to 21 ns for the wrapper. Both are invisible on the ancestor shapes,
// which pay them once for tens of µs of folding.
//
// Together they are now nearly two fifths of a bucket rule — 83 ns of 215 — which is
// a larger share than before, because reading a field by index took the other half
// of that rule away. That is the shape production actually runs: one Eval per rule
// per event, nearly all of them short circuiting immediately.
//
// Read against BenchmarkBucket's finding that merging a bucket into one
// disjunction saves 65 ns per rule: that saving and these are largely the same
// money, since all of it is scaffolding around one Eval, and none of it touches
// what a predicate costs. A bucket rule that costs 215 ns can be made to cost 132,
// and it bottoms out there: what is left is the predicate, which costs cel-go about
// 117 ns wherever it sits, against 29 ns for the closure the compiler plans it into.
// Every trick cel-go has for evaluating a rule faster, taken together and taken past
// what its own documentation sanctions, closes half of what is left between the two
// engines on the shape that matters.
func BenchmarkCELScaffolding(b *testing.B) {
	env, err := NewModelEnv()
	if err != nil {
		b.Fatal(err)
	}

	for _, variant := range celScaffoldingVariants {
		for _, name := range sortedKeys(benchExprs) {
			b.Run(name+"/"+variant.name, func(b *testing.B) {
				event := benchEvent()
				ctx := eval.NewContext(event)
				input := celInput(b, ctx, variant.frame)
				run := variant.build(b, env, benchExprs[name], input)
				b.ReportAllocs()
				for b.Loop() {
					resetContext(ctx, event)
					if run() != types.True {
						b.Fatal("expected a match")
					}
				}
			})
		}

		for _, r := range []int{10, 50, 200} {
			b.Run(fmt.Sprintf("Bucket/%d/%s", r, variant.name), func(b *testing.B) {
				event := scaleEvent()
				ctx := eval.NewContext(event)
				input := celInput(b, ctx, variant.frame)
				runs := make([]func() ref.Val, 0, r)
				for _, rule := range bucketRules(r) {
					runs = append(runs, variant.build(b, env, rule, input))
				}
				b.ReportAllocs()
				for b.Loop() {
					resetContext(ctx, event)
					for _, run := range runs {
						if run() == types.True {
							b.Fatal("no rule should match")
						}
					}
				}
			})
		}
	}
}

// celScaffoldingVariants are the three ways of getting a planned rule
// evaluated, from the one the integration would use today to the one cel-go
// benchmarks itself with.
var celScaffoldingVariants = []struct {
	name  string
	frame bool
	build func(b *testing.B, env *cel.Env, expr string, input any) func() ref.Val
}{
	{name: "Activation", build: programEval},
	{name: "Frame", frame: true, build: programEval},
	{name: "Exec", frame: true, build: interpretableEval},
}

// programEval evaluates through cel.Program, which is what Program returns.
func programEval(b *testing.B, env *cel.Env, expr string, input any) func() ref.Val {
	b.Helper()
	program, err := Program(env, expr, ModelFieldTypes{})
	if err != nil {
		b.Fatal(err)
	}
	return func() ref.Val {
		out, _, err := program.Eval(input)
		if err != nil {
			b.Fatal(err)
		}
		return out
	}
}

// interpretableEval evaluates the planned interpretable directly, which only a
// frame can drive.
func interpretableEval(b *testing.B, env *cel.Env, expr string, input any) func() ref.Val {
	b.Helper()
	plan := planInterpretable(b, env, expr)
	frame := input.(*interpreter.ExecutionFrame)
	return func() ref.Val { return plan.Exec(frame) }
}

// planInterpretable plans a rule the way cel.Env.Program does, stopping at the
// interpretable rather than wrapping it in a cel.Program.
//
// It reproduces newProgram (cel/program.go): a dispatcher holding the bindings
// of every declared function, the environment's own adapter and provider, and the
// decorators planningOptions asks for — the reads first, then Optimize, then the
// regex constants, which is the order cel.Program applies them in.
//
// It runs the optimization pass itself, since Program is what usually does that and
// the point here is to measure the same rule the other variants do.
func planInterpretable(b *testing.B, env *cel.Env, expr string) interpreter.InterpretableV2 {
	b.Helper()
	checked, err := CompileWithTypes(env, expr, ModelFieldTypes{})
	if err != nil {
		b.Fatal(err)
	}
	optimized, _, err := Optimize(env, checked)
	if err != nil {
		b.Fatal(err)
	}

	dispatcher := interpreter.NewDispatcher()
	for _, function := range env.Functions() {
		bindings, err := function.Bindings()
		if err != nil {
			b.Fatal(err)
		}
		if err := dispatcher.Add(bindings...); err != nil {
			b.Fatal(err)
		}
	}

	adapter, provider := env.CELTypeAdapter(), env.CELTypeProvider()
	attributes := interpreter.NewAttributeFactory(env.Container, adapter, provider)
	interp := interpreter.NewInterpreter(dispatcher, env.Container, provider, adapter, attributes)
	plan, err := interp.NewInterpretable(optimized.NativeRep(),
		interpreter.CustomDecoratorV2(readDecorator()),
		interpreter.Optimize(),
		interpreter.CompileRegexConstants(globOptimization, interpreter.MatchesRegexOptimization))
	if err != nil {
		b.Fatal(err)
	}
	return plan
}

// celInput returns what Eval is handed: the activation, or a frame wrapping it.
func celInput(b *testing.B, ctx *eval.Context, frame bool) any {
	b.Helper()
	activation := NewActivation(ctx)
	if !frame {
		return activation
	}
	f, err := interpreter.NewExecutionFrame(activation)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(f.Close)
	return f
}

// BenchmarkScale answers the question the migration turns on: whether the gap
// between the engines is a fixed cost per rule or grows with the size of the
// rule. A production rule set is a large number of rules, each a boolean
// expression, evaluated against every event of its type — so the two scale
// differently and the answer is two numbers, not one.
//
// AllTrue evaluates every predicate. ShortCircuit fails on the first, which is
// what nearly every rule does on nearly every event.
//
// Measured (ns/op, both including the ~30 ns context reset):
//
//	n        SECL All   CEL All    SECL short   CEL short
//	1          170        192          47          194
//	2          276        275          46          197
//	4          418        412          46          201
//	8          860        682          47          205
//	16        1819       1224          47          209
//	22        2227       1581          45          210
//
// Two readings:
//
//   - Per predicate CEL is now the cheaper engine. The slopes over 1..16 are 110 ns
//     for SECL and 69 ns for CEL, and the crossover is at two predicates. A SECL
//     comparison that holds appends to the context's matching subexpression list,
//     which costs it two allocations; a CEL predicate costs one, for boxing the
//     value it read.
//
//   - Per rule they are not close. A rule that short circuits on its first
//     predicate costs SECL 17 ns of evaluation and CEL 164 ns, and that gap is
//     paid by every rule in a bucket on every event, matching or not. None of it is
//     the field: it is what cel-go does around one Eval — the pooled execution
//     frame, the deferred recover, the wrapper — which BenchmarkCELScaffolding
//     measures. Growing the expression barely moves it, 194 ns at one predicate
//     against 210 at sixteen, because a skipped predicate costs about 1 ns.
//
// So what CEL still costs is roughly 150 ns per rule evaluated, and nothing per
// predicate. Whether that is affordable depends on how many rules a bucket holds
// and how many events reach it — BenchmarkBucket measures that shape, and merging a
// bucket into one program now recovers a third of it, since the per-rule
// scaffolding is most of what is left.
var scalePredicates = []string{
	`process.comm == "sh"`,
	`process.uid == 1000`,
	`process.gid == 1001`,
	`process.euid == 1002`,
	`process.egid == 1003`,
	`process.fsuid == 1004`,
	`process.fsgid == 1005`,
	`process.user == "u"`,
	`process.group == "g"`,
	`process.euser == "eu"`,
	`process.egroup == "eg"`,
	`process.fsuser == "fu"`,
	`process.fsgroup == "fg"`,
	`process.pid == 42`,
	`process.tid == 43`,
	`process.ppid == 44`,
	`process.file.name == "bash"`,
	`process.file.path == "/usr/bin/bash"`,
	`process.file.uid == 7`,
	`process.file.gid == 8`,
	`process.file.inode == 9`,
	`process.file.mode == 10`,
}

func scaleEvent() *model.Event {
	e := model.NewFakeEvent()
	p := &e.BaseEvent.ProcessContext.Process
	p.Comm = "sh"
	p.Credentials.UID, p.Credentials.GID = 1000, 1001
	p.Credentials.EUID, p.Credentials.EGID = 1002, 1003
	p.Credentials.FSUID, p.Credentials.FSGID = 1004, 1005
	p.Credentials.User, p.Credentials.Group = "u", "g"
	p.Credentials.EUser, p.Credentials.EGroup = "eu", "eg"
	p.Credentials.FSUser, p.Credentials.FSGroup = "fu", "fg"
	p.PIDContext.Pid, p.PIDContext.Tid = 42, 43
	p.PPid = 44
	p.FileEvent.BasenameStr = "bash"
	p.FileEvent.PathnameStr = "/usr/bin/bash"
	p.FileEvent.FileFields.UID, p.FileEvent.FileFields.GID = 7, 8
	p.FileEvent.FileFields.Inode, p.FileEvent.FileFields.Mode = 9, 10
	return e
}

// all n predicates hold, so all n are evaluated
func andOf(n int) string { return strings.Join(scalePredicates[:n], " && ") }

// the first fails, so the rest are short circuited: what a rule costs when it
// does not match, which is the common case in production
func shortCircuit(n int) string {
	return `process.comm == "zzz" && ` + strings.Join(scalePredicates[:n], " && ")
}

func BenchmarkScale(b *testing.B) {
	for _, n := range []int{1, 2, 4, 8, 16, 22} {
		for _, shape := range []struct {
			name  string
			expr  string
			match bool
		}{
			{"AllTrue", andOf(n), true},
			{"ShortCircuit", shortCircuit(n), false},
		} {
			b.Run(fmt.Sprintf("SECL/%s/%d", shape.name, n), func(b *testing.B) {
				var m model.Model
				rule, err := eval.NewRule("s", shape.expr, ast.NewParsingContext(false), &eval.Opts{})
				if err != nil {
					b.Fatal(err)
				}
				if err := rule.GenEvaluator(&m); err != nil {
					b.Fatal(err)
				}
				event := scaleEvent()
				ctx := eval.NewContext(event)
				b.ReportAllocs()
				for b.Loop() {
					resetContext(ctx, event)
					if rule.Eval(ctx) != shape.match {
						b.Fatal("wrong verdict")
					}
				}
			})

			b.Run(fmt.Sprintf("CEL/%s/%d", shape.name, n), func(b *testing.B) {
				env, err := NewModelEnv()
				if err != nil {
					b.Fatal(err)
				}
				program, err := Program(env, shape.expr, ModelFieldTypes{})
				if err != nil {
					b.Fatal(err)
				}
				event := scaleEvent()
				ctx := eval.NewContext(event)
				activation := NewActivation(ctx)
				b.ReportAllocs()
				for b.Loop() {
					resetContext(ctx, event)
					out, _, err := program.Eval(activation)
					if err != nil {
						b.Fatal(err)
					}
					if (out == types.True) != shape.match {
						b.Fatal("wrong verdict")
					}
				}
			})
		}
	}
}

// BenchmarkBucket measures the shape production actually has: a bucket of rules
// evaluated against one event, none of them matching. BenchmarkScale measures
// one rule; this measures a rule set.
//
// PerRule is the current architecture, one program and one Eval per rule.
// OneProgram is the same rules as a single disjunction, one Eval for the bucket.
//
// Measured (ns/op for the whole bucket, no rule matching):
//
//	rules    SECL per-rule   CEL per-rule   SECL one   CEL one
//	10           221            1644           213       1065
//	50          1203            8264          1029       5090
//	200         6393           36430          5828      23428
//
// CEL is 5 to 6 times dearer, where it was 8 to 10 before a field was read by
// index, and the ratio holds as the bucket grows. Per rule that is 164 ns against
// 22 ns at ten rules, 182 against 32 at two hundred. CEL also allocates once per
// rule where SECL allocates nothing, so a bucket of 200 costs 3.2 kB per event —
// and that allocation is the boxing of the value the rule read.
//
// Evaluating the bucket as one disjunction saves 65 ns per rule, a third of it, and
// that bounds the idea: what it removes is the per-Eval scaffolding, the pooled
// execution frame and the rest of prog.Eval. What is left is the predicate itself,
// which costs cel-go about 117 ns wherever it sits, against 29 ns for the closure
// SECL compiles it into. Merging rules does not make a predicate cheaper.
//
// Note how the per predicate figures differ from BenchmarkScale's: there every
// predicate held, and a SECL comparison that holds appends to the context's
// matching subexpression list, which costs it two allocations and takes it from
// 29 ns to about 105. A predicate that fails — what nearly all of them do — is
// where SECL is furthest ahead.
func bucketRules(r int) []string {
	rules := make([]string, r)
	for i := range rules {
		rules[i] = fmt.Sprintf(`process.comm == "nomatch%d" && process.uid == %d && process.file.name == "x%d"`, i, i, i)
	}
	return rules
}

func BenchmarkBucket(b *testing.B) {
	for _, r := range []int{10, 50, 200} {
		rules := bucketRules(r)
		event := scaleEvent()

		// shape A: one program per rule, one Eval per rule
		b.Run(fmt.Sprintf("CEL/PerRule/%d", r), func(b *testing.B) {
			env, err := NewModelEnv()
			if err != nil {
				b.Fatal(err)
			}
			programs := make([]cel.Program, 0, r)
			for _, rule := range rules {
				p, err := Program(env, rule, ModelFieldTypes{})
				if err != nil {
					b.Fatal(err)
				}
				programs = append(programs, p)
			}
			ctx := eval.NewContext(event)
			activation := NewActivation(ctx)
			b.ReportAllocs()
			for b.Loop() {
				resetContext(ctx, event)
				for _, p := range programs {
					out, _, err := p.Eval(activation)
					if err != nil {
						b.Fatal(err)
					}
					if out == types.True {
						b.Fatal("no rule should match")
					}
				}
			}
		})

		// shape B: the bucket as one disjunction, one Eval
		b.Run(fmt.Sprintf("CEL/OneProgram/%d", r), func(b *testing.B) {
			env, err := NewModelEnv()
			if err != nil {
				b.Fatal(err)
			}
			expr := "(" + strings.Join(rules, ") || (") + ")"
			p, err := Program(env, expr, ModelFieldTypes{})
			if err != nil {
				b.Fatal(err)
			}
			ctx := eval.NewContext(event)
			activation := NewActivation(ctx)
			b.ReportAllocs()
			for b.Loop() {
				resetContext(ctx, event)
				out, _, err := p.Eval(activation)
				if err != nil {
					b.Fatal(err)
				}
				if out == types.True {
					b.Fatal("no rule should match")
				}
			}
		})

		b.Run(fmt.Sprintf("SECL/PerRule/%d", r), func(b *testing.B) {
			var m model.Model
			compiled := make([]*eval.Rule, 0, r)
			for i, rule := range rules {
				cr, err := eval.NewRule(fmt.Sprintf("r%d", i), rule, ast.NewParsingContext(false), &eval.Opts{})
				if err != nil {
					b.Fatal(err)
				}
				if err := cr.GenEvaluator(&m); err != nil {
					b.Fatal(err)
				}
				compiled = append(compiled, cr)
			}
			ctx := eval.NewContext(event)
			b.ReportAllocs()
			for b.Loop() {
				resetContext(ctx, event)
				for _, cr := range compiled {
					if cr.Eval(ctx) {
						b.Fatal("no rule should match")
					}
				}
			}
		})

		b.Run(fmt.Sprintf("SECL/OneProgram/%d", r), func(b *testing.B) {
			var m model.Model
			expr := "(" + strings.Join(rules, ") || (") + ")"
			cr, err := eval.NewRule("all", expr, ast.NewParsingContext(false), &eval.Opts{})
			if err != nil {
				b.Fatal(err)
			}
			if err := cr.GenEvaluator(&m); err != nil {
				b.Fatal(err)
			}
			ctx := eval.NewContext(event)
			b.ReportAllocs()
			for b.Loop() {
				resetContext(ctx, event)
				if cr.Eval(ctx) {
					b.Fatal("no rule should match")
				}
			}
		})
	}
}

var boolSink bool

// BenchmarkFloor bounds what any compilation strategy could be worth, by
// measuring `process.comm == "nomatch"` with one layer of machinery added at a
// time.
//
//	inline, the memory access and the comparison    2.1 ns
//	reached through the context                     2.4 ns
//	a closure pair (the compiler inlines these)     2.4 ns
//	the generated accessor's own closure            3.9 ns
//	SECL's compiled rule, end to end               12.7 ns
//	cel-go, per predicate in a bucket             ~117 ns
//
// The third line is not a real closure call — both closures are defined and
// called in the same function, so they inline away. The fourth is: the accessor
// closure comes back from GetEvaluator as an opaque func value, so an indirect
// call, a pointer chase through the event and a string comparison together cost
// about 4 ns.
//
// Two things follow. Compiling to closures rather than interpreting is worth
// roughly fourteen times, and needs no machine code — SECL already demonstrates
// it. And beyond that there is about 9 ns per predicate left between SECL's
// composed closures and a fused one, which is what generating a closure per
// field and operator would recover. The hard floor underneath both is 2 ns of
// memory access that nothing removes.

// How cheap can `process.comm == "nomatch"` possibly be, evaluated against an
// event through a context? Each step adds one layer of the machinery a compiled
// rule needs, so the differences bound what any compilation strategy can win.
func BenchmarkFloor(b *testing.B) {
	event := scaleEvent()
	ctx := eval.NewContext(event)

	// 1. the memory access and the comparison, inlined: the hard floor
	b.Run("inline", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			boolSink = event.BaseEvent.ProcessContext.Process.Comm == "nomatch0"
		}
	})

	// 2. reached through the context, as any rule must
	b.Run("throughContext", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			boolSink = ctx.Event.(*model.Event).BaseEvent.ProcessContext.Process.Comm == "nomatch0"
		}
	})

	// 3. as a closure pair, which is SECL's compiled shape
	b.Run("closurePair", func(b *testing.B) {
		read := func(c *eval.Context) string {
			return c.Event.(*model.Event).BaseEvent.ProcessContext.Process.Comm
		}
		op := func(a, b string) bool { return a == b }
		const want = "nomatch0"
		predicate := func(c *eval.Context) bool { return op(read(c), want) }
		b.ReportAllocs()
		for b.Loop() {
			boolSink = predicate(ctx)
		}
	})

	// 4. the generated accessor's own closure, as SECL actually calls it
	b.Run("accessorClosure", func(b *testing.B) {
		var m model.Model
		ev, err := m.GetEvaluator("process.comm", "", 0)
		if err != nil {
			b.Fatal(err)
		}
		read := ev.(*eval.StringEvaluator).EvalFnc
		const want = "nomatch0"
		predicate := func(c *eval.Context) bool { return read(c) == want }
		b.ReportAllocs()
		for b.Loop() {
			boolSink = predicate(ctx)
		}
	})

	// 5. SECL's compiled rule, end to end
	b.Run("seclRule", func(b *testing.B) {
		var m model.Model
		rule, err := eval.NewRule("f", `process.comm == "nomatch0"`, ast.NewParsingContext(false), &eval.Opts{})
		if err != nil {
			b.Fatal(err)
		}
		if err := rule.GenEvaluator(&m); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		for b.Loop() {
			boolSink = rule.Eval(ctx)
		}
	})
}

// BenchmarkRead isolates what reading one field costs, which is the question the
// indexed read and readOptimization were built to answer.
//
// Each row adds one layer. Empty is a program that reads nothing, so it is the per
// evaluation floor both engines pay here — the context reset and cel-go's own
// scaffolding. NoopCall is a function of (evt, index) whose implementation returns a
// constant, so it is everything cel-go charges for a call and nothing else: that row
// is what readOptimization removes. ReadInt adds the reader, on a field whose value
// needs no allocation. ReadString adds the boxing of a string into a CEL value.
// TwoReads is the same read twice, so the difference is the marginal cost of a read
// once the surrounding evaluation is paid for.
//
// Measured (ns/op, median of three, including the ~30 ns context reset):
//
//	                      planned        unoptimized
//	Empty                       77                 77
//	NoopCall                   229                229
//	ReadInt                    131                241
//	ReadString                 187                298
//	TwoReads                   139                361
//
// Two readings. A read is around 10 ns of work — the second one in TwoReads costs 8 —
// against the 150 ns cel-go charges for the call that carries it, which is why
// binding the reader when the rule is planned was worth more than the index itself.
// And boxing a string into a CEL value costs 56 ns, more than five times the read: it
// is the largest item left in a scalar rule, and no amount of planning removes it,
// because a two word string cannot travel in a one word interface.
func BenchmarkRead(b *testing.B) {
	env, err := NewModelEnv(cel.Function(benchNoopFunc,
		cel.Overload(benchNoopFunc, []*cel.Type{cel.DynType, cel.IntType}, cel.IntType,
			cel.BinaryBinding(func(ref.Val, ref.Val) ref.Val { return types.Int(benchNoopResult) }))))
	if err != nil {
		b.Fatal(err)
	}

	// The reads are written as the optimization pass would emit them, so that the
	// rows measure the planned form rather than the translation.
	comm, uid := celReaderIndex["process.comm"], celReaderIndex["process.uid"]
	rows := []struct{ name, expr string }{
		{"Empty", `true`},
		{"NoopCall", fmt.Sprintf(`%s(evt, %d) == %d`, benchNoopFunc, uid, benchNoopResult)},
		{"ReadInt", fmt.Sprintf(`%s(evt, %d) == 0`, ReadIntFunc, uid)},
		{"ReadString", fmt.Sprintf(`%s(evt, %d) == "sh"`, ReadStringFunc, comm)},
		{"TwoReads", fmt.Sprintf(`%s(evt, %d) == %s(evt, %d)`, ReadIntFunc, uid, ReadIntFunc, uid)},
	}

	event := benchEvent()
	ctx := eval.NewContext(event)
	activation := NewActivation(ctx)

	for _, row := range rows {
		b.Run(row.name, func(b *testing.B) {
			checked, iss := env.Compile(row.expr)
			if iss.Err() != nil {
				b.Fatal(iss.Err())
			}
			program, err := env.Program(checked, planningOptions()...)
			if err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			for b.Loop() {
				resetContext(ctx, event)
				if _, _, err := program.Eval(activation); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

const (
	// benchNoopFunc is a read that does not read, for the row that measures what a
	// call costs when the work it carries costs nothing.
	benchNoopFunc   = "bench.noop"
	benchNoopResult = 1000
)
