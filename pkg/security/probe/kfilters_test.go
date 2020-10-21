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
	rs := rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(true, SECLConstants, nil))
	addRuleExpr(t, rs, `unlink.filename =~ "/var/log/*" && unlink.filename != "/var/log/datadog/system-probe.log"`)

	if is, _ := isParentPathDiscarder(rs, "unlink", "unlink.filename", "/var/log/datadog/system-probe.log"); is {
		t.Fatal("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(true, SECLConstants, nil))
	addRuleExpr(t, rs, `unlink.filename =~ "/var/log/*" && unlink.filename != "/var/log/datadog/system-probe.log"`)

	if is, _ := isParentPathDiscarder(rs, "unlink", "unlink.filename", "/var/lib/datadog/system-probe.sock"); !is {
		t.Fatal("should be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(true, SECLConstants, nil))
	addRuleExpr(t, rs, `unlink.filename == "/var/log/datadog/system-probe.log"`, `unlink.basename == "datadog"`)

	if is, _ := isParentPathDiscarder(rs, "unlink", "unlink.filename", "/var/log/datadog/datadog-agent.log"); is {
		t.Fatal("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(true, SECLConstants, nil))
	addRuleExpr(t, rs, `unlink.filename =~ "/var/log/*" && unlink.basename =~ ".*"`)

	if is, _ := isParentPathDiscarder(rs, "unlink", "unlink.filename", "/var/lib/.runc/1234"); !is {
		t.Fatal("should be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(true, SECLConstants, nil))
	addRuleExpr(t, rs, `unlink.filename == "/etc/conf.d/httpd.conf" || unlink.basename == "conf.d"`)

	if is, _ := isParentPathDiscarder(rs, "unlink", "unlink.filename", "/etc/conf.d/nginx.conf"); is {
		t.Fatal("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(true, SECLConstants, nil))
	addRuleExpr(t, rs, `unlink.filename == "/etc/conf.d/httpd.conf" || unlink.basename == "sys.d"`)

	if is, _ := isParentPathDiscarder(rs, "unlink", "unlink.filename", "/etc/sys.d/nginx.conf"); is {
		t.Fatal("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(true, SECLConstants, nil))
	addRuleExpr(t, rs, `unlink.basename == "conf.d"`)

	if is, _ := isParentPathDiscarder(rs, "unlink", "unlink.filename", "/etc/conf.d/nginx.conf"); is {
		t.Fatal("shouldn't be a parent discarder")
	}

	// field that doesn't exists shouldn't return any discarders
	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(true, SECLConstants, nil))
	addRuleExpr(t, rs, `rename.old.filename == "/etc/conf.d/abc"`)

	if is, _ := isParentPathDiscarder(rs, "rename", "rename.filename", "/etc/conf.d/nginx.conf"); is {
		t.Fatal("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(true, SECLConstants, nil))
	addRuleExpr(t, rs, `rename.old.filename == "/etc/conf.d/abc"`)

	if is, _ := isParentPathDiscarder(rs, "rename", "rename.old.filename", "/etc/nginx/nginx.conf"); !is {
		t.Fatal("should be a parent discarder")
	}
}
