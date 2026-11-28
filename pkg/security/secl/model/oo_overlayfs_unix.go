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
				// because the field value and this extra input are both compared
				return path
			},
		}
	}

	// OverlayFSPathname handles path evaluations when the file is accessed through an overlayfs mountpoint:
	// the path relative to the mountpoint is compared too, with patterns as globs
	OverlayFSPathname = &eval.OpOverrides{
		GlobPatterns: true,
		ExtraInputs: func(field eval.Field) []*eval.StringEvaluator {
			if !strings.HasSuffix(field, ".file.path") {
				return nil
			}
			return []*eval.StringEvaluator{overlayFSEvaluator(field)}
		},
	}
)
