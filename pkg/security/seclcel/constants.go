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

// arithmeticFolding folds a constant arithmetic call when the rule is planned.
//
// A constant is free, but an expression over constants is not: `O_CREAT|O_TRUNC`
// translates to nested math.bitOr calls, which cel-go's own optimizer leaves alone —
// it folds constant *type conversions* and nothing else. So the flags idiom that a
// third of the real rules are written with would pay two calls on every event.
//
// Decorators run from the bottom up, so a nested expression collapses from the inside
// out and `open.flags & (O_CREAT|O_TRUNC|O_APPEND) > 0` reaches the interpreter as one
// comparison against one integer.
func arithmeticFolding() interpreter.InterpretableDecoratorV2 {
	return func(i interpreter.InterpretableV2) (interpreter.InterpretableV2, error) {
		call, ok := i.(interpreter.InterpretableCall)
		if !ok || !foldableArithmetic[call.Function()] {
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
		if types.IsUnknownOrError(out) {
			// Constant or not, an error belongs to evaluation: leaving the call alone
			// keeps the error where a rule author would look for it.
			return i, nil
		}
		return interpreter.NewConstValue(i.ID(), out), nil
	}
}

// foldableArithmetic is what arithmeticFolding folds: the operators SECL spells with
// a symbol and the math extension implements as a function. The others — comparisons,
// the helpers, anything reading a field — are either free already or not constant.
var foldableArithmetic = map[string]bool{
	bitAndFunc: true,
	bitOrFunc:  true,
	bitXorFunc: true,
	bitNotFunc: true,
}
