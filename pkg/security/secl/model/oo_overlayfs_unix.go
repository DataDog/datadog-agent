// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

// Package model holds model related files
package model

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

var (
	overlayFSEvaluator = func(field eval.Field) *eval.StringEvaluator {
		return &eval.StringEvaluator{
			Field: field,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)

				fileField := strings.TrimSuffix(field, ".path")
				fileEvent, err := ev.GetFileField(fileField)
				if err != nil {
					ev.Error = fmt.Errorf("could not get file field for `%s`: %w", field, err)
					return "" // ideally we should be aborting the evaluation here
				}

				path := ev.FieldHandlers.ResolveFilePath(ev, fileEvent)
				fs := ev.FieldHandlers.ResolveFileFilesystem(ev, fileEvent)
				if fs == "overlay" {
					mountPoint := strings.TrimRight(fileEvent.MountPath, "/")
					return strings.TrimPrefix(path, mountPoint)
				}

				// return the original path if no overlay fs
				// this will cause a non-matching path to be evaluated twice
				// because of the OR operator used in OverlayFSPathname
				return path
			},
		}
	}

	// OverlayFSPathname handles path evaluations when the file is accessed through an overlayfs mountpoint
	OverlayFSPathname = &eval.OpOverrides{
		StringEquals: func(a *eval.StringEvaluator, b *eval.StringEvaluator, state *eval.State) (*eval.BoolEvaluator, error) {
			baseEvaluator, err := eval.GlobCmp.StringEquals(a, b, state)
			if err != nil {
				return nil, err
			}

			if strings.HasSuffix(a.Field, ".file.path") {
				overlayFSPathEvaluator, err := eval.GlobCmp.StringEquals(overlayFSEvaluator(a.Field), b, state)
				if err != nil {
					return nil, err
				}
				return eval.Or(baseEvaluator, overlayFSPathEvaluator, state)
			} else if strings.HasSuffix(b.Field, ".file.path") {
				overlayFSPathEvaluator, err := eval.GlobCmp.StringEquals(overlayFSEvaluator(b.Field), a, state)
				if err != nil {
					return nil, err
				}
				return eval.Or(baseEvaluator, overlayFSPathEvaluator, state)
			}

			return baseEvaluator, nil
		},
		StringValuesContains: func(a *eval.StringEvaluator, b *eval.StringValuesEvaluator, state *eval.State) (*eval.BoolEvaluator, error) {
			baseEvaluator, err := eval.GlobCmp.StringValuesContains(a, b, state)
			if err != nil {
				return nil, err
			}

			if strings.HasSuffix(a.Field, ".file.path") {
				overlayFSPathEvaluator, err := eval.GlobCmp.StringValuesContains(overlayFSEvaluator(a.Field), b, state)
				if err != nil {
					return nil, err
				}
				return eval.Or(baseEvaluator, overlayFSPathEvaluator, state)
			}

			return baseEvaluator, nil
		},
		StringArrayContains: func(a *eval.StringEvaluator, b *eval.StringArrayEvaluator, state *eval.State) (*eval.BoolEvaluator, error) {
			baseEvaluator, err := eval.GlobCmp.StringArrayContains(a, b, state)
			if err != nil {
				return nil, err
			}

			if strings.HasSuffix(a.Field, ".file.path") {
				overlayFSPathEvaluator, err := eval.GlobCmp.StringArrayContains(overlayFSEvaluator(a.Field), b, state)
				if err != nil {
					return nil, err
				}
				return eval.Or(baseEvaluator, overlayFSPathEvaluator, state)
			}

			return baseEvaluator, nil
		},
		StringArrayMatches: func(a *eval.StringArrayEvaluator, b *eval.StringValuesEvaluator, state *eval.State) (*eval.BoolEvaluator, error) {
			return eval.GlobCmp.StringArrayMatches(a, b, state)
		},
	}
)
