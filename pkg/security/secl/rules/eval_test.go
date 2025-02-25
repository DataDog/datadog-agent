// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package rules holds rules related files
package rules

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestIteratorCache(t *testing.T) {
	event := model.NewFakeEvent()

	event.Exec = model.ExecEvent{
		Process: &model.Process{
			FileEvent: model.FileEvent{
				FileFields: model.FileFields{
					UID: 22,
				},
			},
		},
	}
	event.ProcessContext = &model.ProcessContext{
		Ancestor: &model.ProcessCacheEntry{
			ProcessContext: model.ProcessContext{
				Process: model.Process{
					PIDContext: model.PIDContext{
						Pid: 111,
					},
					PPid: 111,
				},
			},
		},
	}

	evalRule, err := eval.NewRule("test", `exec.file.uid == 22 && process.ancestors.pid == 111 && process.ancestors.ppid == 111`, ast.NewParsingContext(false), &eval.Opts{})
	if err != nil {
		t.Error(err)
	}

	rule := &Rule{
		Rule: evalRule,
	}

	err = rule.GenEvaluator(&model.Model{})
	if err != nil {
		t.Error(err)
	}

	ctx := eval.NewContext(event)

	rule.Eval(ctx)

	if len(ctx.IteratorCountCache) != 1 || ctx.IteratorCountCache["BaseEvent.ProcessContext.Ancestor"] != 1 {
		t.Errorf("wrong iterator cache entries: %+v", ctx.IteratorCountCache)
	}
}
