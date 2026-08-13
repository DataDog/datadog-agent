// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package seclcel

// These compare the cost of evaluating the same rule through the SECL evaluator
// and through the translated CEL rule, which is the only way to tell where the
// translation actually costs something.
//
// Read the figures within one table only. Every table here was taken in one
// session on one machine, and the machines differ by more than the effects being
// measured: the same SECL code that reads 267 ns for Comm below read 172 ns in
// the session that produced the figures this file used to record. Comparing a
// row against a number from the git history says nothing. Where a comparison
// matters, both sides of it were measured together.
//
// What a figure contains, beyond evaluating the rule:
//
//   - Resetting the context, which both engines pay here. The rule engine resets
//     once per event and then evaluates the whole rule set, so production
//     amortises it over every rule where this charges it to each one. It is in
//     the loop regardless, because the SECL accessors memoise a resolved array on
//     the context: without a reset, an iterated field is walked on the first
//     iteration and never again, and the benchmark measures a cache instead of a
//     field. It is a larger share of SECL's figures than of CEL's, so leaving it
//     in flatters CEL slightly.
//   - Nothing else measurable. Checking the result is the same either way.
//
// What is left out is building the activation, once per context: the activation
// itself, the root position it hands out, and the execution frame it wraps itself
// in. That is right only if an activation lives as long as the pooled context it
// is bound to, which it may — see the note on the Activation type. If the
// integration ends up building one per event, that cost belongs in these figures.
//
// As measured in one session:
//
//	                 SECL                  CEL
//	Comm             267 ns,  2 allocs     147 ns,   1
//	Path             278 ns,  2            148 ns,   1
//	InList           268 ns,  2            180 ns,   1
//	Regex            420 ns,  2            492 ns,   3
//	Glob             318 ns,  2            281 ns,   2
//	Ancestors       11.9 µs, 12           64.1 µs, 304
//	DeepAncestors   63.6 µs, 304          44.4 µs, 205
//
// Every rule here matches, and that is not a neutral choice: a SECL comparison
// that holds reports itself, and the report is most of the SECL column. Measured
// on the same machine, minus the 32 ns reset, `process.comm == "sh"` costs SECL
// 240 ns when it holds and 9.5 ns when it fails — the difference being
// AddMatchingSubExpr, which boxes both operand values into an interface and
// appends an 88 byte record. CEL records nothing, so the scalar rows are not the
// two engines doing the same work. What SECL spends it on is a feature: it is how
// CWS reports which subexpression matched.
//
// Regex is where CEL is still clearly dearer, and the gap is cel-go's own dispatch
// rather than anything about fields. DeepAncestors is cheaper through CEL, which
// is the cursor paying off: SECL reaches each position by walking from the root
// again.
//
// Where the CEL column came from, in order — each measured in its own session, so
// read the order and not the differences: 672 ns for Comm when a field was resolved
// by joining its path and looking the accessor up by name; 250 ns once the readers
// were generated; 262 ns when the namespace was rooted under one variable; 196 ns
// once a field was read by index; and 147 ns now that a rule is evaluated as an
// interpretable. The three shapes with a literal in them — InList, Regex, Glob —
// would cost 724, 2847 and 736 ns without plannerOptions, since the list, the
// regexp and the glob pattern would be rebuilt on every event.
//
// # What evaluating as an interpretable is worth
//
// A rule is planned into an interpreter.InterpretableV2 and evaluated by handing it
// a frame the activation holds, rather than wrapped in a cel.Program that takes a
// frame from the pool on every call and returns it. The same rules, planned both
// ways in one session:
//
//	                 cel.Program    Rule.Eval    saved
//	Comm                    237          146       91
//	Path                    238          148       89
//	InList                  255          180       75
//	Regex                   548          504       44
//	Glob                    357          279       78
//	Ancestors             63008        63443       none
//	DeepAncestors         43815        44136       none
//	ShortCircuit/22         261          158      102
//	Bucket/10              2022         1197     83/rule
//	Bucket/200            40938        26854     70/rule
//
// It is one constant, which these shapes bracket between 44 and 102 ns. The spread
// is measurement error rather than signal: a rule pays this once whatever it does,
// and nothing in it depends on how a field is read. Allocations are identical — the
// pool was already handing the frame back for free — so it is invisible on the
// ancestor shapes, which pay it once for tens of µs of folding.
//
// What the read by index changed is not this number but the share of a rule it is.
// Against a scalar rule it went from about a fifth to about two fifths, and against
// a bucket rule from a quarter to a third, because the read that surrounded it lost
// around 100 ns. The scaffolding did not grow; the rule around it shrank, and that
// is the shape production runs — one Eval per rule per event, nearly all of them
// short circuiting immediately.
//
// Two thirds of it is the pooled ExecutionFrame — a CPU profile put acquiring and
// releasing it at around a fifth of a scalar evaluation — and the rest is what
// prog.Eval does around the interpretable: a deferred recover, a types.IsError
// test, a three value return. The frame's documentation says it must not be stored,
// which is the question this answers: an Activation holds one for as long as its
// context, and the note there says why that is sound.
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
// # Where the two engines stand
//
//   - A scalar rule that matches is cheaper through CEL — 147 ns against 267 —
//     but that is the match reporting above rather than a win: SECL evaluates the
//     same predicate in about 10 ns and spends the rest recording it. Compare the
//     engines on a rule that does not match and SECL is three times cheaper, 46 ns
//     against 147. That is the honest scalar reading, and BenchmarkScale says the
//     gap is a fixed cost per rule rather than per predicate.
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
//   - A full walk of an iterated field is far dearer through CEL — 64 µs against
//     12 µs on a 150 deep ancestry — because the fold pays the comprehension's own
//     bookkeeping per element where SECL resolves the array once and compares it.
//     It is the largest gap in the file and the one worth attacking next.
//
// The one structural advantage left to SECL beyond that is the per-event
// memoisation BenchmarkSECLWarmCache measures: a second rule mentioning an
// iterated field gets it for free within the same event, where CEL walks it
// again. The same trick would pay here for the boxing, which BenchmarkRead prices
// at 76 ns for a string and which a rule set repeats field by field — see the note
// on what a read is made of.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"

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
			rule, err := NewRule(env, expr, ModelFieldTypes{})
			if err != nil {
				b.Fatal(err)
			}

			event := benchEvent()
			ctx := eval.NewContext(event)
			activation := NewActivation(ctx)
			b.ReportAllocs()
			for b.Loop() {
				resetContext(ctx, event)
				matched, err := rule.Eval(activation)
				if err != nil {
					b.Fatal(err)
				}
				if !matched {
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
// Measured (ns/op, medians of five, both including the context reset):
//
//	n        SECL All   CEL All    SECL short   CEL short
//	1          276        157          46          147
//	2          426        240          47          150
//	4          713        401          46          155
//	8         1415        744          48          159
//	16        2800       1435          46          158
//	22        3529       1793          47          157
//
// Two readings:
//
//   - Per predicate CEL is the cheaper column, and the column is misleading. The
//     slopes over 1..16 are 168 ns for SECL and 85 for CEL, with the crossover at
//     one predicate — but every predicate here holds, and a SECL comparison that
//     holds spends about 230 ns reporting itself, which CEL does not do at all.
//     On evaluation alone SECL is around 10 ns per predicate against CEL's 85. Read
//     this column for how each engine scales, never for which is cheaper.
//
//   - Per rule they are not close, and this is the reading that matters. A rule
//     that short circuits on its first predicate costs SECL 46 ns and CEL 147, a
//     gap paid by every rule in a bucket on every event, matching or not. None of
//     it is the field — BenchmarkRead prices a bound read at 24 ns — and what is
//     left is cel-go's own dispatch. Growing the expression barely moves it, 147 ns
//     at one predicate against 157 at twenty-two, because a skipped predicate costs
//     well under a nanosecond.
//
// So what CEL still costs is roughly 100 ns per rule evaluated over SECL, and
// nothing per predicate. Whether that is affordable depends on how many rules a
// bucket holds and how many events reach it — BenchmarkBucket measures that shape.
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
				rule, err := NewRule(env, shape.expr, ModelFieldTypes{})
				if err != nil {
					b.Fatal(err)
				}
				event := scaleEvent()
				ctx := eval.NewContext(event)
				activation := NewActivation(ctx)
				b.ReportAllocs()
				for b.Loop() {
					resetContext(ctx, event)
					matched, err := rule.Eval(activation)
					if err != nil {
						b.Fatal(err)
					}
					if matched != shape.match {
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
//	10           217            1189           213       1249
//	50          1177            5887          1005       6233
//	200         6351           27124          5863      27871
//
// CEL is 4 to 6 times dearer, where it was 8 to 10 before a field was read by index
// and 5 to 6 before a rule was evaluated as an interpretable. Per rule that is
// 119 ns against 22 at ten rules, 136 against 32 at two hundred. CEL also allocates
// once per rule where SECL allocates nothing, so a bucket of 200 costs 3.2 kB per
// event — and that allocation is the boxing of the value the rule read.
//
// Merging the bucket into one disjunction is now worth nothing, and this is the
// finding that changed when the rule stopped being evaluated through a cel.Program.
// It used to save 65 ns per rule; it now costs 4 to 7. What it saved was the
// per-Eval scaffolding — the pooled execution frame and the rest of prog.Eval —
// which every rule has since shed, and what it costs is its own disjunction. The
// idea is spent: what is left is the predicate itself, which costs cel-go about
// 120 ns wherever it sits, against 29 ns for the closure SECL compiles it into.
// Merging rules never made a predicate cheaper.
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

		// shape A: one rule planned per rule, one Exec per rule
		b.Run(fmt.Sprintf("CEL/PerRule/%d", r), func(b *testing.B) {
			env, err := NewModelEnv()
			if err != nil {
				b.Fatal(err)
			}
			planned := make([]*Rule, 0, r)
			for _, rule := range rules {
				p, err := NewRule(env, rule, ModelFieldTypes{})
				if err != nil {
					b.Fatal(err)
				}
				planned = append(planned, p)
			}
			ctx := eval.NewContext(event)
			activation := NewActivation(ctx)
			b.ReportAllocs()
			for b.Loop() {
				resetContext(ctx, event)
				for _, p := range planned {
					matched, err := p.Eval(activation)
					if err != nil {
						b.Fatal(err)
					}
					if matched {
						b.Fatal("no rule should match")
					}
				}
			}
		})

		// shape B: the bucket as one disjunction, one Exec
		b.Run(fmt.Sprintf("CEL/OneProgram/%d", r), func(b *testing.B) {
			env, err := NewModelEnv()
			if err != nil {
				b.Fatal(err)
			}
			expr := "(" + strings.Join(rules, ") || (") + ")"
			p, err := NewRule(env, expr, ModelFieldTypes{})
			if err != nil {
				b.Fatal(err)
			}
			ctx := eval.NewContext(event)
			activation := NewActivation(ctx)
			b.ReportAllocs()
			for b.Loop() {
				resetContext(ctx, event)
				matched, err := p.Eval(activation)
				if err != nil {
					b.Fatal(err)
				}
				if matched {
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
// Measured (ns/op, median of three, including the ~32 ns context reset):
//
//	Empty              34
//	NoopCall          168      a call cel-go plans as a call:  +134
//	ReadInt            58      the read, bound to its reader:   +24
//	ReadString        134      boxing the string it read:       +76
//	TwoReads           71      a second read of the same field: +12
//
// Two readings. A read is around 24 ns of work, and 12 once the surrounding
// evaluation is paid for, against the 134 ns cel-go charges for a call it has not
// bound — which is why binding the reader when the rule is planned was worth more
// than the index itself. An earlier session measured the same rows with the binding
// off at 241, 298 and 361 ns against 131, 187 and 139 with it on, which is the same
// statement from the other side.
//
// And boxing a string into a CEL value costs 76 ns, three times the read: it is the
// largest item left in a scalar rule, and no amount of planning removes it, because
// a two word string cannot travel in a one word interface. It is per read, so a
// bucket that reads the same field in rule after rule pays it every time — the one
// saving still on this side of the ref.Val boundary, and the same shape as the
// per-event cache BenchmarkSECLWarmCache measures for SECL.
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
			// Planned the way NewRule plans a rule, but from CEL source rather than
			// SECL, since these rows are written as the optimization pass emits them.
			plan, err := planRule(env, checked)
			if err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			for b.Loop() {
				resetContext(ctx, event)
				if out := plan.Exec(activation.frame); types.IsError(out) {
					b.Fatal(out)
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
