// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

var (
	pathEvaluator = &eval.StringEvaluator{
		EvalFnc: func(ctx *eval.Context) string {
			return (*Event)(ctx.Object).Exec.Process.SymlinkPathnameStr
		},
	}

	baseEvaluator = &eval.StringEvaluator{
		EvalFnc: func(ctx *eval.Context) string {
			return (*Event)(ctx.Object).Exec.Process.SymlinkBasenameStr
		},
	}

	// ProcessSymlinkPathname handles symlink for process enrtries
	ProcessSymlinkPathname = &eval.OpOverrides{
		StringEquals: func(a *eval.StringEvaluator, b *eval.StringEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			path, err := eval.GlobCmp.StringEquals(a, b, opts, state)
			if err != nil {
				return nil, err
			}

			// currently only override exec events
			if a.Field == "exec.file.path" {
				symlink, err := eval.GlobCmp.StringEquals(pathEvaluator, b, opts, state)
				if err != nil {
					return nil, err
				}
				return eval.Or(path, symlink, opts, state)
			} else if b.Field == "exec.file.path" {
				symlink, err := eval.GlobCmp.StringEquals(a, pathEvaluator, opts, state)
				if err != nil {
					return nil, err
				}
				return eval.Or(path, symlink, opts, state)
			}

			return eval.GlobCmp.StringEquals(a, b, opts, state)
		},
		StringValuesContains: func(a *eval.StringEvaluator, b *eval.StringValuesEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			path, err := eval.GlobCmp.StringValuesContains(a, b, opts, state)
			if err != nil {
				return nil, err
			}

			// currently only override exec events
			if a.Field == "exec.file.path" {
				symlink, err := eval.GlobCmp.StringValuesContains(pathEvaluator, b, opts, state)
				if err != nil {
					return nil, err
				}
				return eval.Or(path, symlink, opts, state)
			}

			return eval.GlobCmp.StringValuesContains(a, b, opts, state)
		},
		StringArrayContains: func(a *eval.StringEvaluator, b *eval.StringArrayEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			path, err := eval.GlobCmp.StringArrayContains(a, b, opts, state)
			if err != nil {
				return nil, err
			}

			// currently only override exec events
			if a.Field == "exec.file.path" {
				symlink, err := eval.GlobCmp.StringArrayContains(pathEvaluator, b, opts, state)
				if err != nil {
					return nil, err
				}
				return eval.Or(path, symlink, opts, state)
			}

			return eval.GlobCmp.StringArrayContains(a, b, opts, state)
		},
		StringArrayMatches: func(a *eval.StringArrayEvaluator, b *eval.StringValuesEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			return eval.GlobCmp.StringArrayMatches(a, b, opts, state)
		},
	}

	// ProcessSymlinkBasename handles symlink for process enrtries
	ProcessSymlinkBasename = &eval.OpOverrides{
		StringEquals: func(a *eval.StringEvaluator, b *eval.StringEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			path, err := eval.StringEquals(a, b, opts, state)
			if err != nil {
				return nil, err
			}

			// currently only override exec events
			if a.Field == "exec.file.name" {
				symlink, err := eval.StringEquals(baseEvaluator, b, opts, state)
				if err != nil {
					return nil, err
				}
				return eval.Or(path, symlink, opts, state)
			} else if b.Field == "exec.file.name" {
				symlink, err := eval.StringEquals(a, baseEvaluator, opts, state)
				if err != nil {
					return nil, err
				}
				return eval.Or(path, symlink, opts, state)
			}

			return eval.StringEquals(a, b, opts, state)
		},
		StringValuesContains: func(a *eval.StringEvaluator, b *eval.StringValuesEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			path, err := eval.StringValuesContains(a, b, opts, state)
			if err != nil {
				return nil, err
			}

			// currently only override exec events
			if a.Field == "exec.file.name" {
				symlink, err := eval.StringValuesContains(baseEvaluator, b, opts, state)
				if err != nil {
					return nil, err
				}
				return eval.Or(path, symlink, opts, state)
			}

			return eval.StringValuesContains(a, b, opts, state)
		},
		StringArrayContains: func(a *eval.StringEvaluator, b *eval.StringArrayEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			path, err := eval.StringArrayContains(a, b, opts, state)
			if err != nil {
				return nil, err
			}

			// currently only override exec events
			if a.Field == "exec.file.name" {
				symlink, err := eval.StringArrayContains(baseEvaluator, b, opts, state)
				if err != nil {
					return nil, err
				}
				return eval.Or(path, symlink, opts, state)
			}

			return eval.StringArrayContains(a, b, opts, state)
		},
		StringArrayMatches: func(a *eval.StringArrayEvaluator, b *eval.StringValuesEvaluator, opts *eval.Opts, state *eval.State) (*eval.BoolEvaluator, error) {
			return eval.StringArrayMatches(a, b, opts, state)
		},
	}
)
