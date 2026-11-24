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

	// ProcessSymlinkPathname handles symlink for process entries: the symlink paths of the executed file are
	// compared too, with patterns as globs
	ProcessSymlinkPathname = &eval.OpOverrides{
		GlobPatterns: true,
		ExtraInputs: func(field eval.Field) []*eval.StringEvaluator {
			// currently only override exec events
			if field != "exec.file.path" && field != "process.file.path" {
				return nil
			}
			return []*eval.StringEvaluator{symlinkPathnameEvaluators[0](field), symlinkPathnameEvaluators[1](field)}
		},
	}

	// ProcessSymlinkBasename handles symlink for process entries: the symlink basename of the executed file is
	// compared too
	ProcessSymlinkBasename = &eval.OpOverrides{
		ExtraInputs: func(field eval.Field) []*eval.StringEvaluator {
			// currently only override exec events
			if field != "exec.file.name" && field != "process.file.name" {
				return nil
			}
			return []*eval.StringEvaluator{symlinkBasenameEvaluator(field)}
		},
	}
)
