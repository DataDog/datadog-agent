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
// As measured: a direct struct read is a little cheaper through CEL, a field
// resolved by a handler about a third dearer, and an iterated field several times
// dearer. The last is the one that matters, and the cause is structural rather
// than incidental: an implicit comparison against an array field becomes an
// exists() over element positions, so one array read turns into a length read
// plus one read per element, each of which walks the ancestor chain from the
// root. SECL resolves the whole array in a single walk.

import (
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
	// four segments plus an iterated field
	"Ancestors": `process.ancestors.comm == "sshd"`,
}

func benchEvent() *model.Event {
	event := model.NewFakeEvent()
	event.BaseEvent.ProcessContext.Process.FileEvent.PathnameStr = "/usr/bin/bash"
	event.BaseEvent.ProcessContext.Process.Comm = "sh"
	ancestry(event, []string{"bash", "sshd"}, []uint32{1000, 0})
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

			ctx := eval.NewContext(benchEvent())
			b.ReportAllocs()
			for b.Loop() {
				if !rule.Eval(ctx) {
					b.Fatal("expected a match")
				}
			}
		})
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

			activation := NewActivation(eval.NewContext(benchEvent()))
			b.ReportAllocs()
			for b.Loop() {
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
