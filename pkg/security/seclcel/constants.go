// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

import (
	"sort"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/interpreter"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// constantDeclarations declares the names SECL rules use for the kernel's own
// constants — `O_CREAT`, `AF_INET`, `BPF_PROG_LOAD`, a thousand more.
//
// They are declared as constants rather than variables, so each one is a value the
// planner reads straight out of the expression and an event pays nothing for it. The
// model already holds them as evaluators over a fixed value, so the type and the
// value both come from there and cannot drift from what SECL resolves.
//
// `true` and `false` are in the same table and are skipped: they are CEL keywords,
// and the translation already emits them as literals.
func constantDeclarations() []cel.EnvOption {
	constants := model.SECLConstants()

	names := make([]string, 0, len(constants))
	for name := range constants {
		names = append(names, name)
	}
	sort.Strings(names)

	opts := make([]cel.EnvOption, 0, len(names))
	for _, name := range names {
		switch constant := constants[name].(type) {
		case *eval.IntEvaluator:
			opts = append(opts, cel.Constant(name, cel.IntType, types.Int(constant.Value)))
		case *eval.StringEvaluator:
			opts = append(opts, cel.Constant(name, cel.StringType, types.String(constant.Value)))
		case *eval.BoolEvaluator:
			continue
		case *eval.StringArrayEvaluator:
			opts = append(opts, cel.Constant(name, cel.ListType(cel.StringType), stringsToVal(constant.Values)))
		case *eval.IntArrayEvaluator:
			opts = append(opts, cel.Constant(name, cel.ListType(cel.IntType), intsToVal(constant.Values)))
		}
	}
	return opts
}

// constantFolding evaluates a call over literals when the rule is planned.
//
// A constant is free, but an expression over constants is not: `O_CREAT|O_TRUNC`
// translates to nested math.bitOr calls, which cel-go's own optimizer leaves alone —
// it folds constant *type conversions* and nothing else. So the flags idiom that a
// third of the real rules are written with would pay two calls on every event, and a
// list of patterns would be compiled on every one.
//
// Decorators run from the bottom up, and cel-go's own list folding runs before this
// one — see plannerOptions — so a nested expression collapses from the inside out:
// `open.flags & (O_CREAT|O_TRUNC) > 0` reaches the interpreter as one comparison
// against one integer, and `secl.patterns([secl.glob("/etc/*"), …])` as one prepared
// set.
func constantFolding() interpreter.InterpretableDecoratorV2 {
	return func(i interpreter.InterpretableV2) (interpreter.InterpretableV2, error) {
		call, ok := i.(interpreter.InterpretableCall)
		if !ok {
			return i, nil
		}
		constructor, foldable := foldableCall(call.Function())
		if !foldable {
			return i, nil
		}

		for _, arg := range call.Args() {
			if _, isConst := arg.(interpreter.InterpretableConst); !isConst {
				return i, nil
			}
		}

		// Every argument is a literal, so the call has nothing to read from an event
		// and its result is the same for every one of them.
		out := i.Eval(interpreter.EmptyActivation())
		if failure, failed := out.(*types.Err); failed {
			if !constructor {
				// An arithmetic error belongs to evaluation: leaving the call alone
				// keeps it where a rule author would look for it.
				return i, nil
			}
			// A pattern that does not compile is something else — a rule that cannot
			// work rather than one that answers an error per event — and reporting it
			// here is what makes it a rule that does not load, which is where SECL
			// reports it too.
			return nil, failure.Unwrap()
		}
		if types.IsUnknownOrError(out) {
			return i, nil
		}
		return interpreter.NewConstValue(i.ID(), out), nil
	}
}

// foldableCall reports whether a call over literals is worth collapsing, and whether
// it builds a value — which is what decides where an error belongs.
//
// The two groups are the operators SECL spells with a symbol and the math extension
// implements as a function, and the constructors that turn a literal into a compiled
// matcher. Everything else is either free already, or reads an event.
//
// GlobFunc is here for its unary overload, the one that builds a value; the binary one
// takes a field as its first argument and so is never constant. Its pattern is compiled
// at planning time either way — see globOptimization.
func foldableCall(function string) (constructor bool, foldable bool) {
	switch function {
	case bitAndFunc, bitOrFunc, bitXorFunc, bitNotFunc:
		return false, true
	case GlobFunc, RegexpFunc, PatternsFunc:
		return true, true
	}
	return false, false
}
