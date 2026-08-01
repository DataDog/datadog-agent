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
//	Comm             169 ns,  2 allocs    672 ns,    6     291 ns,   1
//	Path             179 ns,  2           846 ns,    8     399 ns,   2
//	InList           163 ns,  2                          — 314 ns,   1
//	Regex            351 ns,  2                          — 681 ns,   4
//	Glob             257 ns,  2                          — 480 ns,   3
//	Ancestors        6.6 µs, 12          18.8 µs,  178    1.86 µs,   9
//	DeepAncestors   55.7 µs, 304          150 µs, 1066    48.2 µs, 205
//
// The three shapes with no before column were added with planningOptions, which
// is what makes them affordable: unplanned they cost 724 ns, 2847 ns and 736 ns,
// because the list literal, the regexp and the glob pattern were rebuilt on
// every event.
//
// Decomposing what is left of a scalar read, the 291 ns of Comm: 30 ns is
// resetting the context, which SECL pays too; 47 ns is cel-go evaluating a
// program that reads nothing at all; 7 ns is the activation handing back a
// cached root; 37 ns is the node lookup and the reader, of which the struct
// read is 3 ns and boxing the result 2 ns; and the remaining ~170 ns is cel-go
// resolving the attribute and dispatching the comparison. Almost none of it is
// the model, which is why the readers cannot close the gap on their own.
//
// The iterated cases are what the node graph was built for. Presenting an
// iterated field as a list of positions meant reading its length — a full walk
// of the ancestry — before the predicate saw the first element, and then
// resolving each position by walking from the root again, so reading positions
// 0, 1, 2… cost O(N²). A cursor yields the elements, so Ancestors stops at the
// second of 150 rather than paying for all of them first.
//
// Where the two engines now stand:
//
//   - A scalar read is dearer through CEL, by about 120 ns, and the breakdown
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
	"testing"

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
	comms[1] = "sshd"

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
