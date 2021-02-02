// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/hashicorp/golang-lru/simplelru"
)

func addRuleExpr(t *testing.T, rs *rules.RuleSet, exprs ...string) {
	var ruleDefs []*rules.RuleDefinition

	for i, expr := range exprs {
		ruleDef := &rules.RuleDefinition{
			ID:         fmt.Sprintf("ID%d", i),
			Expression: expr,
			Tags:       make(map[string]string),
		}
		ruleDefs = append(ruleDefs, ruleDef)
	}

	if err := rs.AddRules(ruleDefs); err != nil {
		t.Fatal(err)
	}
}

func TestIsParentDiscarder(t *testing.T) {
	regexCache, err := simplelru.NewLRU(64, nil)
	if err != nil {
		t.Fatal(err)
	}

	rs := rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(SECLConstants, nil))
	addRuleExpr(t, rs, `unlink.filename =~ "/var/log/*" && unlink.filename != "/var/log/datadog/system-probe.log"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, FileUnlinkEventType, "unlink.filename", "/var/log/datadog/system-probe.log"); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(SECLConstants, nil))
	addRuleExpr(t, rs, `unlink.filename =~ "/var/log/*" && unlink.filename != "/var/log/datadog/system-probe.log"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, FileUnlinkEventType, "unlink.filename", "/var/lib/datadog/system-probe.sock"); !is {
		t.Error("should be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(SECLConstants, nil))
	addRuleExpr(t, rs, `unlink.filename == "/var/log/datadog/system-probe.log"`, `unlink.basename == "datadog"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, FileUnlinkEventType, "unlink.filename", "/var/log/datadog/datadog-agent.log"); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(SECLConstants, nil))
	addRuleExpr(t, rs, `unlink.filename =~ "/var/log/*" && unlink.basename =~ ".*"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, FileUnlinkEventType, "unlink.filename", "/var/lib/.runc/1234"); !is {
		t.Error("should be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(SECLConstants, nil))
	addRuleExpr(t, rs, `unlink.filename == "/etc/conf.d/httpd.conf" || unlink.basename == "conf.d"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, FileUnlinkEventType, "unlink.filename", "/etc/conf.d/nginx.conf"); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(SECLConstants, nil))
	addRuleExpr(t, rs, `unlink.filename == "/etc/conf.d/httpd.conf" || unlink.basename == "sys.d"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, FileUnlinkEventType, "unlink.filename", "/etc/sys.d/nginx.conf"); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(SECLConstants, nil))
	addRuleExpr(t, rs, `unlink.basename == "conf.d"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, FileUnlinkEventType, "unlink.filename", "/etc/conf.d/nginx.conf"); is {
		t.Error("shouldn't be a parent discarder")
	}

	// field that doesn't exists shouldn't return any discarders
	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(SECLConstants, nil))
	addRuleExpr(t, rs, `rename.old.filename == "/etc/conf.d/abc"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, FileRenameEventType, "rename.filename", "/etc/conf.d/nginx.conf"); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(SECLConstants, nil))
	addRuleExpr(t, rs, `rename.old.filename == "/etc/conf.d/abc"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, FileRenameEventType, "rename.old.filename", "/etc/nginx/nginx.conf"); !is {
		t.Error("should be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(SECLConstants, nil))
	addRuleExpr(t, rs, `unlink.filename =~ "/etc/conf.d/*"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, FileUnlinkEventType, "unlink.filename", "/etc/sys.d/nginx.conf"); !is {
		t.Error("should be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(SECLConstants, nil))
	addRuleExpr(t, rs, `unlink.filename =~ "*/conf.*"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, FileUnlinkEventType, "unlink.filename", "/etc/conf.d/abc"); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(SECLConstants, nil))
	addRuleExpr(t, rs, `unlink.filename =~ "/etc/conf.d/ab*"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, FileUnlinkEventType, "unlink.filename", "/etc/conf.d/abc"); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(SECLConstants, nil))
	addRuleExpr(t, rs, `unlink.filename =~ "*/conf.d/ab*"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, FileUnlinkEventType, "unlink.filename", "/etc/conf.d/abc"); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(SECLConstants, nil))
	addRuleExpr(t, rs, `unlink.filename =~ "*/conf.d"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, FileUnlinkEventType, "unlink.filename", "/etc/conf.d/abc"); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(SECLConstants, nil))
	addRuleExpr(t, rs, `unlink.filename =~ "/etc/*"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, FileUnlinkEventType, "unlink.filename", "/etc/cron.d/log"); is {
		t.Error("shouldn't be a parent discarder")
	}
}

func TestApproverAncestors(t *testing.T) {
	rs := rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(SECLConstants, nil))
	addRuleExpr(t, rs, `open.filename == "/etc/passwd" && process.ancestors.name == "vipw"`, `open.filename == "/etc/shadow" && process.ancestors.name == "vipw"`)

	capabilities, exists := allCapabilities["open"]
	if !exists {
		t.Fatal("no capabilities for open")
	}

	approvers, err := rs.GetApprovers("open", capabilities.GetFieldCapabilities())
	if err != nil {
		t.Fatal(err)
	}

	if values, exists := approvers["open.filename"]; !exists || len(values) != 2 {
		t.Fatalf("expected approver not found: %v", values)
	}
}
