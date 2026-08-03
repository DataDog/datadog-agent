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
// What is left out is building the CEL activation: 108 ns and 144 B, once. That
// is right only if an activation lives as long as the pooled context it is bound
// to, which it may — see the note on the activation type. If the integration
// ends up building one per event, that cost belongs in these figures, and the
// activation's root cache needs re-measuring against it: the cache is what makes
// the activation 144 B rather than 8.
//
// As measured on one machine, before and after the generated readers replaced
// resolving a field by joining its path and looking the accessor up by name:
//
//	                 SECL              CEL before          CEL now
//	Comm             177 ns,  2 allocs    672 ns,    6     250 ns,   1
//	Path             189 ns,  2           846 ns,    8     330 ns,   2
//	InList           172 ns,  2                          — 271 ns,   1
//	Regex            360 ns,  2                          — 607 ns,   4
//	Glob             274 ns,  2                          — 404 ns,   3
//	Ancestors        6.5 µs, 12          18.8 µs,  178    1.70 µs,   9
//	DeepAncestors   56.3 µs, 304          150 µs, 1066    41.9 µs, 205
//
// The three shapes with no before column were added with planningOptions, which
// is what makes them affordable: unplanned they cost 724 ns, 2847 ns and 736 ns,
// because the list literal, the regexp and the glob pattern were rebuilt on
// every event.
//
// Decomposing what is left of a scalar read, the 250 ns of Comm: 30 ns is
// resetting the context, which SECL pays too; 47 ns is cel-go evaluating a
// program that reads nothing at all; 7 ns is the activation handing back a
// cached root; 47 ns is the reader, of which the struct read is 3 ns and boxing
// the result 2 ns; and the remaining ~120 ns is cel-go resolving the attribute
// and dispatching the comparison. Almost none of it is the model, which is why
// the readers cannot close the gap on their own.
//
// A CPU profile puts the rest of it where nothing here can reach: acquiring and
// releasing the pooled ExecutionFrame is around a fifth of a scalar evaluation.
// cel-go will accept a frame passed to Eval instead of taking one from the pool,
// but its documentation says a frame must not be stored, so that is a question
// for whoever wires up the rule engine rather than a change to make here.
//
// The same profile is what found the type adapter: every read returns a ref.Val
// already, and the interpreter adapted it anyway, through a type switch dozens
// of cases long, to produce the same value. seclAdapter is that one case, and
// it was worth 6% of a scalar read and 8% of an iterated one.
//
// Of an iterated evaluation, the fold itself — not the predicate — is most of
// the cost. Profiling foldIterable: 41% the step (accu || predicate), 28% the
// condition (!accu, which dispatches through runtime overload matching on every
// element), and 22% the cursor and the element value. The condition is pure
// bookkeeping: over a quarter of the work is the comprehension asking itself
// whether to continue.
//
// Path gains more than Comm from binding at planning time because it selects
// twice, and it is per select that the work went away.
//
// The iterated cases are what the cursors were built for. Presenting an
// iterated field as a list of positions meant reading its length — a full walk
// of the ancestry — before the predicate saw the first element, and then
// resolving each position by walking from the root again, so reading positions
// 0, 1, 2… cost O(N²). A cursor yields the elements, so Ancestors stops at the
// second of 150 rather than paying for all of them first.
//
// Where the two engines now stand:
//
//   - A scalar read is dearer through CEL, by about 90 ns, and the breakdown
//     above says where: cel-go's own attribute resolution and dispatch. SECL
//     compiles a rule down to a closure over the field, so it has none of that
//     to do.
//   - A shallow match on a deep iterated field is dearer through SECL, because
//     an array field is resolved whole — 150 strings, 9 kB — before anything
//     compares them, where a cursor stops at the element that matched.
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

	// "sshd" is the second ancestor, so the shallow benchmarks stop early while
	// the deep one runs to the end of the chain.
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
//	1          164        251          45          249
//	2          257        387          46          255
//	4          421        660          46          260
//	8          852       1214          45          266
//	16        1733       2369          47          271
//
// Two readings:
//
//   - Per predicate the engines are close. The slopes over 1..16 are 105 ns for
//     SECL and 141 ns for CEL: CEL costs about 36 ns more per predicate, 1.34x,
//     and it does not degrade as the expression grows. On allocations CEL is
//     ahead — 1 per predicate against SECL's 2, because SECL appends to its
//     matching subexpression list on every comparison that holds.
//
//   - Per rule they are not close. A rule that short circuits on its first
//     predicate costs SECL 15 ns of evaluation and CEL 220 ns, and that gap is
//     paid by every rule in the bucket on every event, matching or not. It is
//     the interpreter's own scaffolding: acquiring and releasing the pooled
//     ExecutionFrame is about 50 ns of it, the rest is attribute resolution and
//     dispatch. Growing the expression barely moves it — 249 ns at one
//     predicate, 271 ns at sixteen — because the skipped predicates cost about
//     1 ns each.
//
// So the cost of CEL is roughly 200 ns per rule evaluated, not per predicate.
// Whether that is affordable depends on how many rules a bucket holds and how
// many events reach it — BenchmarkBucket measures that shape, and shows that
// merging a bucket into one program recovers only a quarter of the difference,
// because most of it is the predicate rather than the Eval around it.
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
//	10           219            2196           209       1785
//	50          1150           11516          1040       8674
//	200         6335           48936          5766      35796
//
// CEL is 8 to 10 times dearer, and the ratio holds as the bucket grows. Per
// rule that is 220 ns against 22 ns at ten rules, 245 against 32 at two hundred.
// CEL also allocates once per rule where SECL allocates nothing, so a bucket of
// 200 costs 3.2 kB per event.
//
// Evaluating the bucket as one disjunction saves 66 ns per rule — a quarter —
// and no more. That is worth knowing because it bounds the idea: what it removes
// is the per-Eval scaffolding, the pooled execution frame and the rest of
// prog.Eval. What is left is the predicate itself, and a predicate costs cel-go
// about 179 ns wherever it sits, against 29 ns for the closure SECL compiles it
// into. Merging rules does not make a predicate cheaper.
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
//	cel-go, per predicate in a bucket             ~179 ns
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
