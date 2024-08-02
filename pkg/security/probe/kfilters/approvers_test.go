// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package kfilters holds kfilters related files
package kfilters

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func newFakeEvent() eval.Event {
	return model.NewFakeEvent()
}

func TestApproverAncestors1(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	ruleOpts, evalOpts := rules.NewBothOpts(enabled)

	rs := rules.NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
	rules.AddTestRuleExpr(t, rs, `open.file.path == "/etc/passwd" && process.ancestors.file.name == "vipw"`, `open.file.path == "/etc/shadow" && process.ancestors.file.name == "vipw"`)

	capabilities, exists := allCapabilities["open"]
	if !exists {
		t.Fatal("no capabilities for open")
	}

	approvers, err := rs.GetEventTypeApprovers("open", capabilities)
	if err != nil {
		t.Fatal(err)
	}

	if values, exists := approvers["open.file.path"]; !exists || len(values) != 2 {
		t.Fatalf("expected approver not found: %v", values)
	}
}

func TestApproverAncestors2(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	ruleOpts, evalOpts := rules.NewBothOpts(enabled)

	rs := rules.NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
	rules.AddTestRuleExpr(t, rs, `(open.file.path == "/etc/shadow" || open.file.path == "/etc/gshadow") && process.ancestors.file.path not in ["/usr/bin/dpkg"]`)
	capabilities, exists := allCapabilities["open"]
	if !exists {
		t.Fatal("no capabilities for open")
	}
	approvers, err := rs.GetEventTypeApprovers("open", capabilities)
	if err != nil {
		t.Fatal(err)
	}
	if values, exists := approvers["open.file.path"]; !exists || len(values) != 2 {
		t.Fatalf("expected approver not found: %v", values)
	}
}

func TestApproverGlob(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	ruleOpts, evalOpts := rules.NewBothOpts(enabled)

	rs := rules.NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
	rules.AddTestRuleExpr(t, rs, `open.file.path =~ "/var/run/secrets/eks.amazonaws.com/serviceaccount/*/token" && process.file.path not in ["/bin/kubectl"]`)
	capabilities, exists := allCapabilities["open"]
	if !exists {
		t.Fatal("no capabilities for open")
	}
	approvers, err := rs.GetEventTypeApprovers("open", capabilities)
	if err != nil {
		t.Fatal(err)
	}
	if values, exists := approvers["open.file.path"]; !exists || len(values) != 1 {
		t.Fatalf("expected approver not found: %v", values)
	}
}

func TestApproverFlags(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	ruleOpts, evalOpts := rules.NewBothOpts(enabled)

	rs := rules.NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
	rules.AddTestRuleExpr(t, rs, `open.flags & (O_SYNC | O_NOCTTY) > 0`)
	capabilities, exists := allCapabilities["open"]
	if !exists {
		t.Fatal("no capabilities for open")
	}
	approvers, err := rs.GetEventTypeApprovers("open", capabilities)
	if err != nil {
		t.Fatal(err)
	}
	if values, exists := approvers["open.flags"]; !exists || len(values) != 1 {
		t.Fatalf("expected approver not found: %v", values)
	}
}

func TestApproverWildcardBasename(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	ruleOpts, evalOpts := rules.NewBothOpts(enabled)

	rs := rules.NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
	rules.AddTestRuleExpr(t, rs, `open.file.path =~ "/var/run/secrets/*"`)
	capabilities, exists := allCapabilities["open"]
	if !exists {
		t.Fatal("no capabilities for open")
	}
	approvers, err := rs.GetEventTypeApprovers("open", capabilities)
	if err != nil {
		t.Fatal(err)
	}
	if values, exists := approvers["open.file.path"]; exists || len(values) != 0 {
		t.Fatalf("unexpected approver found: %v", values)
	}
}

func TestApproverAUIDRange(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	ruleOpts, evalOpts := rules.NewBothOpts(enabled)

	assert := func(t *testing.T, ruleDefs []string, min, max uint32) {
		t.Helper()

		rs := rules.NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
		rules.AddTestRuleExpr(t, rs, ruleDefs...)

		capabilities, exists := allCapabilities["open"]
		if !exists {
			t.Fatal("no capabilities for open")
		}
		approvers, err := rs.GetEventTypeApprovers("open", capabilities)
		if err != nil {
			t.Fatal(err)
		}
		if values, exists := approvers["process.auid"]; !exists {
			t.Fatalf("expected approver not found: %+v", values)
		}

		kfilters, err := KFilterGetters["open"](approvers)
		if err != nil {
			t.Fatal(err)
		}
		if len(kfilters) != 1 {

			if min != 0 && max != 0 {
				t.Fatalf("expected kfilter not found: %+v", kfilters)
			} else {
				// no kfilter expected
				return
			}
		}

		key := makeEntryKey(auidRangeApproversTable, model.FileOpenEventType)
		entry := kfilters[key]
		if entry == nil {
			t.Fatalf("expected kfilter not found: %+v => %+v", key, kfilters)
		}

		value := entry.(*hashEntry).value.(*ebpf.UInt32RangeMapItem)
		if value.Min != min || value.Max != max {
			t.Fatalf("expected kfilter not found: %+v => %+v", kfilters, value)
		}
	}

	assert(t, []string{`open.file.path =~ "/tmp/*" && process.auid > 1000 && process.auid < 2000`}, 0, 1999)
	assert(t, []string{`open.file.path =~ "/tmp/*" && process.auid > 1000`}, 1001, maxAUID)
	assert(t, []string{`open.file.path =~ "/tmp/*" && process.auid < 1000`}, 0, 999)
	assert(t, []string{`open.file.path =~ "/tmp/*" && process.auid >= 1000 && process.auid <= 2000`}, 0, 2000)
	assert(t, []string{`open.file.path =~ "/tmp/*" && process.auid >= 1000`}, 1000, maxAUID)
	assert(t, []string{`open.file.path =~ "/tmp/*" && process.auid <= 1000`}, 0, 1000)

	assert(t, []string{
		`open.file.path =~ "/tmp/*" && process.auid > 1000`,
		`open.file.path =~ "/tmp/*" && process.auid < 500`,
	}, 0, 0)
	assert(t, []string{
		`open.file.path =~ "/tmp/*" && process.auid >= 1000`,
		`open.file.path =~ "/tmp/*" && process.auid > 1500`,
	}, 1000, maxAUID)
	assert(t, []string{
		`open.file.path =~ "/tmp/*" && process.auid < 1000`,
		`open.file.path =~ "/tmp/*" && process.auid < 500`,
	}, 0, 999)
}
