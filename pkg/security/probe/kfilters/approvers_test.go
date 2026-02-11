// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package kfilters holds kfilters related files
package kfilters

import (
	"maps"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/stretchr/testify/assert"

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

	approvers, _, _, err := rs.GetEventTypeApprovers("open", capabilities)
	if err != nil {
		t.Fatal(err)
	}

	if values, exists := approvers["open.file.path"]; !exists || len(values) != 2 {
		t.Fatalf("expected approver not found: %v", values)
	}

	var hasPasswdApprover, hasShadowApprover bool
	for _, value := range approvers["open.file.path"] {
		if value.Value.(string) == "/etc/passwd" {
			if value.Type != eval.ScalarValueType {
				t.Fatalf("expected ScalarValueType, got %v", value.Type)
			}
			hasPasswdApprover = true
		}
		if value.Value.(string) == "/etc/shadow" {
			if value.Type != eval.ScalarValueType {
				t.Fatalf("expected ScalarValueType, got %v", value.Type)
			}
			hasShadowApprover = true
		}
	}

	assert.Truef(t, hasPasswdApprover, "expected passwd approver not found")
	assert.Truef(t, hasShadowApprover, "expected shadow approver not found")
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
	approvers, _, _, err := rs.GetEventTypeApprovers("open", capabilities)
	if err != nil {
		t.Fatal(err)
	}
	if values, exists := approvers["open.file.path"]; !exists || len(values) != 2 {
		t.Fatalf("expected approver not found: %v", values)
	}

	var hasShadowApprover, hasGshadowApprover bool
	for _, value := range approvers["open.file.path"] {
		if value.Value.(string) == "/etc/shadow" {
			if value.Type != eval.ScalarValueType {
				t.Fatalf("expected ScalarValueType, got %v", value.Type)
			}
			hasShadowApprover = true
		}
		if value.Value.(string) == "/etc/gshadow" {
			if value.Type != eval.ScalarValueType {
				t.Fatalf("expected ScalarValueType, got %v", value.Type)
			}
			hasGshadowApprover = true
		}
	}

	assert.Truef(t, hasShadowApprover, "expected shadow approver not found")
	assert.Truef(t, hasGshadowApprover, "expected gshadow approver not found")
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
	approvers, _, _, err := rs.GetEventTypeApprovers("open", capabilities)
	if err != nil {
		t.Fatal(err)
	}
	if values, exists := approvers["open.file.path"]; !exists || len(values) != 1 {
		t.Fatalf("expected approver not found: %v", values)
	}

	valueString, ok := approvers["open.file.path"][0].Value.(string)
	if !ok {
		t.Fatalf("expected string value, got %v", approvers["open.file.path"][0].Value)
	}

	assert.Equal(t, "/var/run/secrets/eks.amazonaws.com/serviceaccount/*/token", valueString)
	assert.Equal(t, eval.GlobValueType, approvers["open.file.path"][0].Type)
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
	approvers, _, _, err := rs.GetEventTypeApprovers("open", capabilities)
	if err != nil {
		t.Fatal(err)
	}
	if values, exists := approvers["open.flags"]; !exists || len(values) != 1 {
		t.Fatalf("expected approver not found: %v", values)
	}

	valueInt, ok := approvers["open.flags"][0].Value.(int)
	if !ok {
		t.Fatalf("expected int value, got %v", approvers["open.flags"][0].Value)
	}

	assert.Equal(t, unix.O_SYNC|unix.O_NOCTTY, valueInt, "expected flags O_SYNC|O_NOCTTY, got %d", valueInt)
	assert.Equal(t, eval.BitmaskValueType, approvers["open.flags"][0].Type)
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
	approvers, _, _, err := rs.GetEventTypeApprovers("open", capabilities)
	if err != nil {
		t.Fatal(err)
	}
	if values, exists := approvers["open.file.path"]; exists || len(values) != 0 {
		t.Fatalf("unexpected approver found: %v", values)
	}
}

func TestApproverInUpperLayer(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	ruleOpts, evalOpts := rules.NewBothOpts(enabled)

	t.Run("in_upper_layer-ok-1", func(t *testing.T) {
		rs := rules.NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
		rules.AddTestRuleExpr(t, rs, `open.file.in_upper_layer`)
		capabilities, exists := allCapabilities["open"]
		if !exists {
			t.Fatal("no capabilities for open")
		}
		approvers, _, _, err := rs.GetEventTypeApprovers("open", capabilities)
		if err != nil {
			t.Fatal(err)
		}
		if values, exists := approvers["open.file.in_upper_layer"]; !exists || len(values) != 1 {
			t.Fatalf("expected approver not found: %v", values)
		}
		valueBool, ok := approvers["open.file.in_upper_layer"][0].Value.(bool)
		if !ok {
			t.Fatalf("expected bool value, got %v", approvers["open.file.in_upper_layer"][0].Value)
		}
		assert.Truef(t, valueBool, "expected in_upper_layer approver to be true")
	})

	t.Run("in_upper_layer-ok-2", func(t *testing.T) {
		rs := rules.NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
		rules.AddTestRuleExpr(t, rs, `open.file.in_upper_layer == true`)
		capabilities, exists := allCapabilities["open"]
		if !exists {
			t.Fatal("no capabilities for open")
		}
		approvers, _, _, err := rs.GetEventTypeApprovers("open", capabilities)
		if err != nil {
			t.Fatal(err)
		}
		if values, exists := approvers["open.file.in_upper_layer"]; !exists || len(values) != 1 {
			t.Fatalf("expected approver not found: %v", values)
		}
		valueBool, ok := approvers["open.file.in_upper_layer"][0].Value.(bool)
		if !ok {
			t.Fatalf("expected bool value, got %v", approvers["open.file.in_upper_layer"][0].Value)
		}
		assert.Truef(t, valueBool, "expected in_upper_layer approver to be true")
	})

	t.Run("in_upper_layer-ko", func(t *testing.T) {
		rs := rules.NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
		rules.AddTestRuleExpr(t, rs, `!open.file.in_upper_layer`)
		capabilities, exists := allCapabilities["open"]
		if !exists {
			t.Fatal("no capabilities for open")
		}
		approvers, _, _, err := rs.GetEventTypeApprovers("open", capabilities)
		if err != nil {
			t.Fatal(err)
		}
		if values, exists := approvers["open.file.in_upper_layer"]; exists || len(values) != 0 {
			t.Fatalf("expected approver not found: %v", values)
		}
	})
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
		approvers, _, _, err := rs.GetEventTypeApprovers("open", capabilities)
		if err != nil {
			t.Fatal(err)
		}
		if values, exists := approvers["process.auid"]; !exists {
			t.Fatalf("expected approver not found: %+v", values)
		}

		kfilters, _, err := KFilterGetters["open"](approvers)
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

		key := makeKFilterKey(auidRangeApproversTable, model.FileOpenEventType)
		entry := kfilters[key]
		if entry == nil {
			t.Fatalf("expected kfilter not found: %+v => %+v", key, kfilters)
		}

		value := entry.(*hashKFilter).value.(*ebpf.UInt32RangeMapItem)
		if value.Min != min || value.Max != max {
			t.Fatalf("expected kfilter not found: %+v => %+v", kfilters, value)
		}
	}

	assert(t, []string{`open.file.path =~ "/tmp/*" && process.auid > 1000 && process.auid < 2000`}, 0, maxAUID)
	assert(t, []string{`open.file.path =~ "/tmp/*" && process.auid > 1000`}, 1001, maxAUID)
	assert(t, []string{`open.file.path =~ "/tmp/*" && process.auid < 1000`}, 0, 999)
	assert(t, []string{`open.file.path =~ "/tmp/*" && process.auid >= 1000 && process.auid <= 2000`}, 0, maxAUID)
	assert(t, []string{`open.file.path =~ "/tmp/*" && process.auid >= 1000`}, 1000, maxAUID)
	assert(t, []string{`open.file.path =~ "/tmp/*" && process.auid <= 1000`}, 0, 1000)

	assert(t, []string{
		`open.file.path =~ "/tmp/*" && process.auid > 1000`,
		`open.file.path =~ "/tmp/*" && process.auid < 500`,
	}, 0, maxAUID)
	assert(t, []string{
		`open.file.path =~ "/tmp/*" && process.auid >= 1000`,
		`open.file.path =~ "/tmp/*" && process.auid > 1500`,
	}, 1000, maxAUID)
	assert(t, []string{
		`open.file.path =~ "/tmp/*" && process.auid < 1000`,
		`open.file.path =~ "/tmp/*" && process.auid < 500`,
	}, 0, 999)
	assert(t, []string{
		`open.file.path =~ "/tmp/*" && process.auid != AUDIT_AUID_UNSET`,
	}, 0, maxAUID)
}

func TestApproversConnect(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	ruleOpts, evalOpts := rules.NewBothOpts(enabled)

	testCases := []struct {
		name            string
		ruleExpressions []string
		assertionsCb    func(_ *testing.T, _ *rules.RuleSet, _ rules.Approvers)
	}{
		{
			name:            "addr-family-af-inet",
			ruleExpressions: []string{`connect.addr.family == AF_INET`},
			assertionsCb: func(t *testing.T, _ *rules.RuleSet, approvers rules.Approvers) {
				values, exists := approvers["connect.addr.family"]
				if !exists || len(values) != 1 {
					t.Fatal("expected approver values not found")
				}

				if values[0].Value != unix.AF_INET {
					t.Fatalf("expected AF_INET, got %v", values[0].Value)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rs := rules.NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
			for _, expr := range tc.ruleExpressions {
				rules.AddTestRuleExpr(t, rs, expr)
			}

			capabilities, exists := allCapabilities["connect"]
			if !exists {
				t.Fatal("no capabilities for connect")
			}

			approvers, _, _, err := rs.GetEventTypeApprovers("connect", capabilities)
			if err != nil {
				t.Fatal(err)
			}

			tc.assertionsCb(t, rs, approvers)
		})
	}
}

func TestApproversSetSockOpt(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	ruleOpts, evalOpts := rules.NewBothOpts(enabled)

	testCases := []struct {
		name            string
		ruleExpressions []string
		assertionsCb    func(_ *testing.T, _ *rules.RuleSet, _ rules.Approvers)
	}{
		{
			name:            "setsockopt-level",
			ruleExpressions: []string{`setsockopt.level == SOL_SOCKET`},
			assertionsCb: func(t *testing.T, _ *rules.RuleSet, approvers rules.Approvers) {
				values, exists := approvers["setsockopt.level"]
				if !exists || len(values) != 1 {
					t.Fatal("expected approver values not found")
				}

				if values[0].Value != unix.SOL_SOCKET {
					t.Fatalf("expected SOL_SOCKET, got %v", values[0].Value)
				}
			},
		},
		{
			name:            "setsockopt-optname",
			ruleExpressions: []string{`setsockopt.optname == SO_ATTACH_FILTER`},
			assertionsCb: func(t *testing.T, _ *rules.RuleSet, approvers rules.Approvers) {
				values, exists := approvers["setsockopt.optname"]
				if !exists || len(values) != 1 {
					t.Fatal("expected approver values not found")
				}

				if values[0].Value != unix.SO_ATTACH_FILTER {
					t.Fatalf("expected SO_ATTACH_FILTER, got %v", values[0].Value)
				}
			},
		},
		{
			name:            "setsockopt-level-and-optname",
			ruleExpressions: []string{`setsockopt.level == SOL_SOCKET`, `setsockopt.optname == SO_ATTACH_FILTER`},
			assertionsCb: func(t *testing.T, _ *rules.RuleSet, approvers rules.Approvers) {
				levelValues, levelExists := approvers["setsockopt.level"]
				if !levelExists || len(levelValues) != 1 {
					t.Fatal("expected level approver values not found")
				}

				if levelValues[0].Value != unix.SOL_SOCKET {
					t.Fatalf("expected SOL_SOCKET, got %v", levelValues[0].Value)
				}
				optnameValues, optnameExists := approvers["setsockopt.optname"]
				if !optnameExists || len(optnameValues) != 1 {
					t.Fatal("expected optname approver values not found")
				}

				if optnameValues[0].Value != unix.SO_ATTACH_FILTER {
					t.Fatalf("expected SO_ATTACH_FILTER, got %v", optnameValues[0].Value)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rs := rules.NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
			for _, expr := range tc.ruleExpressions {
				rules.AddTestRuleExpr(t, rs, expr)
			}

			capabilities, exists := allCapabilities["setsockopt"]
			if !exists {
				t.Fatal("no capabilities for setsockopt")
			}

			approvers, _, _, err := rs.GetEventTypeApprovers("setsockopt", capabilities)
			if err != nil {
				t.Fatal(err)
			}

			tc.assertionsCb(t, rs, approvers)
		})
	}
}

func TestApproverFlagsRdOnlyScalar(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	ruleOpts, evalOpts := rules.NewBothOpts(enabled)

	rs := rules.NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
	rules.AddTestRuleExpr(t, rs, `open.flags == O_RDONLY`)
	capabilities, exists := allCapabilities["open"]
	if !exists {
		t.Fatal("no capabilities for open")
	}
	approvers, _, _, err := rs.GetEventTypeApprovers("open", capabilities)
	if err != nil {
		t.Fatal(err)
	}
	if values, exists := approvers["open.flags"]; !exists || len(values) != 1 {
		t.Fatalf("expected approver not found: %v", values)
	}

	valueInt, ok := approvers["open.flags"][0].Value.(int)
	if !ok {
		t.Fatalf("expected int value, got %v", approvers["open.flags"][0].Value)
	}

	assert.Equal(t, unix.O_RDONLY, valueInt, "expected O_RDONLY, got %d", valueInt)
	assert.Equal(t, eval.ScalarValueType, approvers["open.flags"][0].Type)
}

func TestApproverFlagsRdOnlyWithMask(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	ruleOpts, evalOpts := rules.NewBothOpts(enabled)

	rs := rules.NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
	rules.AddTestRuleExpr(t, rs, `open.flags & O_ACCMODE == O_RDONLY`)
	capabilities, exists := allCapabilities["open"]
	if !exists {
		t.Fatal("no capabilities for open")
	}
	approvers, _, _, err := rs.GetEventTypeApprovers("open", capabilities)
	if err != nil {
		t.Fatal(err)
	}
	if values, exists := approvers["open.flags"]; !exists || len(values) != 1 {
		t.Fatalf("expected approver not found: %v", values)
	}

	valueInt, ok := approvers["open.flags"][0].Value.(int)
	if !ok {
		t.Fatalf("expected int value, got %v", approvers["open.flags"][0].Value)
	}

	assert.Equal(t, unix.O_RDONLY, valueInt, "expected O_RDONLY, got %d", valueInt)
	assert.Equal(t, eval.ScalarValueType, approvers["open.flags"][0].Type)
}

func TestApproverFlagsMixedScalarAndMask(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	ruleOpts, evalOpts := rules.NewBothOpts(enabled)

	rs := rules.NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
	rules.AddTestRuleExpr(t, rs, `(open.flags & O_CREAT > 0)`, `(open.flags & O_ACCMODE == O_RDONLY)`)

	capabilities, exists := allCapabilities["open"]
	if !exists {
		t.Fatal("no capabilities for open")
	}
	approvers, _, _, err := rs.GetEventTypeApprovers("open", capabilities)
	if err != nil {
		t.Fatal(err)
	}

	if values, exists := approvers["open.flags"]; !exists || len(values) != 2 {
		t.Fatalf("expected 2 approvers, found: %v", values)
	}

	// Should have both a bitmask approver for O_CREAT and a scalar approver for O_RDONLY
	var hasBitmaskApprover, hasScalarApprover bool
	for _, value := range approvers["open.flags"] {
		if value.Type == eval.BitmaskValueType {
			hasBitmaskApprover = true
			if value.Value.(int) != unix.O_CREAT {
				t.Fatalf("expected O_CREAT bitmask approver, got %v", value.Value)
			}
		}
		if value.Type == eval.ScalarValueType {
			hasScalarApprover = true
			if value.Value != unix.O_RDONLY {
				t.Fatalf("expected O_RDONLY scalar approver, got %v", value.Value)
			}
		}
	}

	assert.Truef(t, hasBitmaskApprover, "expected bitmask approver for O_CREAT")
	assert.Truef(t, hasScalarApprover, "expected scalar approver for O_RDONLY")
}

func TestApproverFlagsZeroValueFlag(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	ruleOpts, evalOpts := rules.NewBothOpts(enabled)

	rs := rules.NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
	rules.AddTestRuleExpr(t, rs, `open.flags & (O_CREAT|O_RDONLY) > 0`)
	capabilities, exists := allCapabilities["open"]
	if !exists {
		t.Fatal("no capabilities for open")
	}
	approvers, _, _, err := rs.GetEventTypeApprovers("open", capabilities)
	if err != nil {
		t.Fatal(err)
	}
	if values, exists := approvers["open.flags"]; !exists || len(values) != 1 {
		t.Fatalf("expected approver not found: %v", values)
	}

	valueInt, ok := approvers["open.flags"][0].Value.(int)
	if !ok {
		t.Fatalf("expected int value, got %v", approvers["open.flags"][0].Value)
	}

	assert.Equal(t, unix.O_CREAT, valueInt, "expected O_CREAT, got %d", valueInt)
	assert.Equal(t, eval.BitmaskValueType, approvers["open.flags"][0].Type)
}

func TestLastApproverEventType(t *testing.T) {
	approversCopy := maps.Clone(KFilterGetters)

	for eventType := model.FirstEventType; eventType <= model.LastApproverEventType; eventType++ {
		delete(approversCopy, eventType.String())
	}

	var approverTypesNotInRange []string

	for eventType := range approversCopy {
		approverTypesNotInRange = append(approverTypesNotInRange, eventType)
	}

	assert.Len(t, approverTypesNotInRange, 0, "event types %q are not part the [FirstEventType; LastApproverEventType] range", approverTypesNotInRange)
}
