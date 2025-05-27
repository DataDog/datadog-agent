// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package kfilters holds kfilters related files
package kfilters

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestDiscarderReport(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	ruleOpts, evalOpts := rules.NewBothOpts(enabled)
	ruleOpts.WithSupportedDiscarders(map[eval.Field]bool{
		"open.file.path": true,
	})

	cfg := &config.Config{
		EnableKernelFilters: true,
		EnableApprovers:     true,
	}

	t.Run("one-field-ok", func(t *testing.T) {
		rs := rules.NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
		rules.AddTestRuleExpr(t, rs, `open.file.path == "/etc/passwd"`)

		report, err := ComputeFilters(cfg, rs)
		if err != nil {
			t.Fatal(err)
		}

		if len(report.DiscardersReport.Supported) != 1 {
			t.Errorf("expected 1 supported discarder, got %d", len(report.DiscardersReport.Supported))
		}
	})

	t.Run("two-field-ko", func(t *testing.T) {
		rs := rules.NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
		rules.AddTestRuleExpr(t, rs, `open.file.path == "/etc/passwd"`)
		rules.AddTestRuleExpr(t, rs, `open.file.name == "group"`)

		report, err := ComputeFilters(cfg, rs)
		if err != nil {
			t.Fatal(err)
		}

		if len(report.DiscardersReport.Supported) != 0 {
			t.Errorf("expected 0 supported discarder, got %d", len(report.DiscardersReport.Supported))
		}

		if len(report.DiscardersReport.Invalid) != 1 {
			t.Errorf("expected 1 invalid discarder, got %d", len(report.DiscardersReport.Invalid))
		}
	})
}
