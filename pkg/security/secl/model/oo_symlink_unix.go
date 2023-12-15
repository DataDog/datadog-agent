// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

// Package model holds model related files
package model

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

var (
	symlinkPathnameEvaluators = [MaxSymlinks]func(field eval.Field) *eval.StringEvaluator{
		func(field eval.Field) *eval.StringEvaluator {
			return &eval.StringEvaluator{
				Field: field,
				EvalFnc: func(ctx *eval.Context) string {
					if path := ctx.Event.(*Event).ProcessContext.SymlinkPathnameStr[0]; path != "" {
						return path
					}
					return ctx.Event.(*Event).ProcessContext.FileEvent.PathnameStr
				},
			}
		},
		func(field eval.Field) *eval.StringEvaluator {
			return &eval.StringEvaluator{
				Field: field,
				EvalFnc: func(ctx *eval.Context) string {
					if path := ctx.Event.(*Event).ProcessContext.SymlinkPathnameStr[1]; path != "" {
						return path
					}
					return ctx.Event.(*Event).ProcessContext.FileEvent.PathnameStr
				},
			}
		},
	}

	symlinkBasenameEvaluator = func(field eval.Field) *eval.StringEvaluator {
		return &eval.StringEvaluator{
			Field: field,
			EvalFnc: func(ctx *eval.Context) string {
				if name := ctx.Event.(*Event).ProcessContext.SymlinkBasenameStr; name != "" {
					return name
				}
				return ctx.Event.(*Event).ProcessContext.FileEvent.BasenameStr
			},
		}
	}

	// ProcessSymlinkPathname handles symlink for process enrtries
	ProcessSymlinkPathname = &eval.OpOverrides{
		StringEquals: func(a *eval.StringEvaluator, b *eval.StringEvaluator, state *eval.State) (*eval.BoolEvaluator, error) {
			path, err := eval.GlobCmp.StringEquals(a, b, state)
			if err != nil {
				return nil, err
			}

			// currently only override exec events
			if a.Field == "exec.file.path" || a.Field == "process.file.path" {
				se1, err := eval.GlobCmp.StringEquals(symlinkPathnameEvaluators[0](a.Field), b, state)
				if err != nil {
					return nil, err
				}

				se2, err := eval.GlobCmp.StringEquals(symlinkPathnameEvaluators[1](a.Field), b, state)
				if err != nil {
					return nil, err
				}

				or, err := eval.Or(se1, se2, state)
				if err != nil {
					return nil, err
				}

				return eval.Or(path, or, state)
			} else if b.Field == "exec.file.path" || b.Field == "process.file.path" {
				se1, err := eval.GlobCmp.StringEquals(symlinkPathnameEvaluators[0](b.Field), a, state)
				if err != nil {
					return nil, err
				}

				se2, err := eval.GlobCmp.StringEquals(symlinkPathnameEvaluators[1](b.Field), a, state)
				if err != nil {
					return nil, err
				}

				or, err := eval.Or(se1, se2, state)
				if err != nil {
					return nil, err
				}

				return eval.Or(path, or, state)
			}

			return path, nil
		},
		StringValuesContains: func(a *eval.StringEvaluator, b *eval.StringValuesEvaluator, state *eval.State) (*eval.BoolEvaluator, error) {
			path, err := eval.GlobCmp.StringValuesContains(a, b, state)
			if err != nil {
				return nil, err
			}

			// currently only override exec events
			if a.Field == "exec.file.path" || a.Field == "process.file.path" {
				se1, err := eval.GlobCmp.StringValuesContains(symlinkPathnameEvaluators[0](a.Field), b, state)
				if err != nil {
					return nil, err
				}
				se2, err := eval.GlobCmp.StringValuesContains(symlinkPathnameEvaluators[1](a.Field), b, state)
				if err != nil {
					return nil, err
				}
				or, err := eval.Or(se1, se2, state)
				if err != nil {
					return nil, err
				}

				return eval.Or(path, or, state)
			}

			return path, nil
		},
		StringArrayContains: func(a *eval.StringEvaluator, b *eval.StringArrayEvaluator, state *eval.State) (*eval.BoolEvaluator, error) {
			path, err := eval.GlobCmp.StringArrayContains(a, b, state)
			if err != nil {
				return nil, err
			}

			// currently only override exec events
			if a.Field == "exec.file.path" || a.Field == "process.file.path" {
				se1, err := eval.GlobCmp.StringArrayContains(symlinkPathnameEvaluators[0](a.Field), b, state)
				if err != nil {
					return nil, err
				}
				se2, err := eval.GlobCmp.StringArrayContains(symlinkPathnameEvaluators[1](a.Field), b, state)
				if err != nil {
					return nil, err
				}
				or, err := eval.Or(se1, se2, state)
				if err != nil {
					return nil, err
				}

				return eval.Or(path, or, state)
			}

			return path, nil
		},
		StringArrayMatches: func(a *eval.StringArrayEvaluator, b *eval.StringValuesEvaluator, state *eval.State) (*eval.BoolEvaluator, error) {
			return eval.GlobCmp.StringArrayMatches(a, b, state)
		},
	}

	// ProcessSymlinkBasename handles symlink for process enrtries
	ProcessSymlinkBasename = &eval.OpOverrides{
		StringEquals: func(a *eval.StringEvaluator, b *eval.StringEvaluator, state *eval.State) (*eval.BoolEvaluator, error) {
			path, err := eval.StringEquals(a, b, state)
			if err != nil {
				return nil, err
			}

			// currently only override exec events
			if a.Field == "exec.file.name" || a.Field == "process.file.name" {
				symlink, err := eval.StringEquals(symlinkBasenameEvaluator(a.Field), b, state)
				if err != nil {
					return nil, err
				}
				return eval.Or(path, symlink, state)
			} else if b.Field == "exec.file.name" || b.Field == "process.file.name" {
				symlink, err := eval.StringEquals(a, symlinkBasenameEvaluator(b.Field), state)
				if err != nil {
					return nil, err
				}
				return eval.Or(path, symlink, state)
			}

			return path, nil
		},
		StringValuesContains: func(a *eval.StringEvaluator, b *eval.StringValuesEvaluator, state *eval.State) (*eval.BoolEvaluator, error) {
			path, err := eval.StringValuesContains(a, b, state)
			if err != nil {
				return nil, err
			}

			// currently only override exec events
			if a.Field == "exec.file.name" || a.Field == "process.file.name" {
				symlink, err := eval.StringValuesContains(symlinkBasenameEvaluator(a.Field), b, state)
				if err != nil {
					return nil, err
				}
				return eval.Or(path, symlink, state)
			}

			return path, nil
		},
		StringArrayContains: func(a *eval.StringEvaluator, b *eval.StringArrayEvaluator, state *eval.State) (*eval.BoolEvaluator, error) {
			path, err := eval.StringArrayContains(a, b, state)
			if err != nil {
				return nil, err
			}

			// currently only override exec events
			if a.Field == "exec.file.name" || a.Field == "process.file.name" {
				symlink, err := eval.StringArrayContains(symlinkBasenameEvaluator(a.Field), b, state)
				if err != nil {
					return nil, err
				}
				return eval.Or(path, symlink, state)
			}

			return path, nil
		},
		StringArrayMatches: func(a *eval.StringArrayEvaluator, b *eval.StringValuesEvaluator, state *eval.State) (*eval.BoolEvaluator, error) {
			return eval.StringArrayMatches(a, b, state)
		},
	}
)
