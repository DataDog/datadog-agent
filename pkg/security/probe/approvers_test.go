// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"testing"

	seclog "github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestApproverAncestors1(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	var opts rules.Opts
	opts.
		WithConstants(model.SECLConstants).
		WithEventTypeEnabled(enabled).
		WithLegacyFields(model.SECLLegacyFields).
		WithLogger(&seclog.PatternLogger{})

	m := &model.Model{}
	rs := rules.NewRuleSet(m, m.NewEvent, &opts)
	addRuleExpr(t, rs, `open.file.path == "/etc/passwd" && process.ancestors.file.name == "vipw"`, `open.file.path == "/etc/shadow" && process.ancestors.file.name == "vipw"`)

	capabilities, exists := allCapabilities["open"]
	if !exists {
		t.Fatal("no capabilities for open")
	}

	approvers, err := rs.GetEventApprovers("open", capabilities.GetFieldCapabilities())
	if err != nil {
		t.Fatal(err)
	}

	if values, exists := approvers["open.file.path"]; !exists || len(values) != 2 {
		t.Fatalf("expected approver not found: %v", values)
	}
}

func TestApproverAncestors2(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	var opts rules.Opts
	opts.
		WithConstants(model.SECLConstants).
		WithEventTypeEnabled(enabled).
		WithLegacyFields(model.SECLLegacyFields).
		WithLogger(&seclog.PatternLogger{})

	m := &model.Model{}
	rs := rules.NewRuleSet(m, m.NewEvent, &opts)
	addRuleExpr(t, rs, `(open.file.path == "/etc/shadow" || open.file.path == "/etc/gshadow") && process.ancestors.file.path not in ["/usr/bin/dpkg"]`)
	capabilities, exists := allCapabilities["open"]
	if !exists {
		t.Fatal("no capabilities for open")
	}
	approvers, err := rs.GetEventApprovers("open", capabilities.GetFieldCapabilities())
	if err != nil {
		t.Fatal(err)
	}
	if values, exists := approvers["open.file.path"]; !exists || len(values) != 2 {
		t.Fatalf("expected approver not found: %v", values)
	}
}
