// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/log"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
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

	enabled := map[eval.EventType]bool{"*": true}

	rs := rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(model.SECLConstants, nil, enabled, nil, model.SECLLegacyAttributes, &log.PatternLogger{}))
	addRuleExpr(t, rs, `unlink.file.path =~ "/var/log/*" && unlink.file.path != "/var/log/datadog/system-probe.log"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, model.FileUnlinkEventType, "unlink.file.path", "/var/log/datadog/system-probe.log"); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(model.SECLConstants, nil, enabled, nil, model.SECLLegacyAttributes, &log.PatternLogger{}))
	addRuleExpr(t, rs, `unlink.file.path =~ "/var/log/*" && unlink.file.path != "/var/log/datadog/system-probe.log"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, model.FileUnlinkEventType, "unlink.file.path", "/var/lib/datadog/system-probe.sock"); !is {
		t.Error("should be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(model.SECLConstants, nil, enabled, nil, model.SECLLegacyAttributes, &log.PatternLogger{}))
	addRuleExpr(t, rs, `unlink.file.path == "/var/log/datadog/system-probe.log"`, `unlink.file.name == "datadog"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, model.FileUnlinkEventType, "unlink.file.path", "/var/log/datadog/datadog-agent.log"); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(model.SECLConstants, nil, enabled, nil, model.SECLLegacyAttributes, &log.PatternLogger{}))
	addRuleExpr(t, rs, `unlink.file.path =~ "/var/log/*" && unlink.file.name =~ ".*"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, model.FileUnlinkEventType, "unlink.file.path", "/var/lib/.runc/1234"); is {
		t.Error("shouldn't be able to find a parent discarder, due to partial evaluation: true && unlink.file.name =~ '.*'")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(model.SECLConstants, nil, enabled, nil, model.SECLLegacyAttributes, &log.PatternLogger{}))
	addRuleExpr(t, rs, `unlink.file.path == "/etc/conf.d/httpd.conf" || unlink.file.name == "conf.d"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, model.FileUnlinkEventType, "unlink.file.path", "/etc/conf.d/nginx.conf"); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(model.SECLConstants, nil, enabled, nil, model.SECLLegacyAttributes, &log.PatternLogger{}))
	addRuleExpr(t, rs, `unlink.file.path == "/etc/conf.d/httpd.conf" || unlink.file.name == "sys.d"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, model.FileUnlinkEventType, "unlink.file.path", "/etc/sys.d/nginx.conf"); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(model.SECLConstants, nil, enabled, nil, model.SECLLegacyAttributes, &log.PatternLogger{}))
	addRuleExpr(t, rs, `unlink.file.name == "conf.d"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, model.FileUnlinkEventType, "unlink.file.path", "/etc/conf.d/nginx.conf"); is {
		t.Error("shouldn't be a parent discarder")
	}

	// field that doesn't exists shouldn't return any discarders
	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(model.SECLConstants, nil, enabled, nil, model.SECLLegacyAttributes, &log.PatternLogger{}))
	addRuleExpr(t, rs, `rename.file.path == "/etc/conf.d/abc"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, model.FileRenameEventType, "rename.file.path", "/etc/conf.d/nginx.conf"); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(model.SECLConstants, nil, enabled, nil, model.SECLLegacyAttributes, &log.PatternLogger{}))
	addRuleExpr(t, rs, `rename.file.path == "/etc/conf.d/abc"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, model.FileRenameEventType, "rename.file.path", "/etc/nginx/nginx.conf"); !is {
		t.Error("should be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(model.SECLConstants, nil, enabled, nil, model.SECLLegacyAttributes, &log.PatternLogger{}))
	addRuleExpr(t, rs, `unlink.file.path =~ "/etc/conf.d/*"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, model.FileUnlinkEventType, "unlink.file.path", "/etc/sys.d/nginx.conf"); !is {
		t.Error("should be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(model.SECLConstants, nil, enabled, nil, model.SECLLegacyAttributes, &log.PatternLogger{}))
	addRuleExpr(t, rs, `unlink.file.path =~ "*/conf.*"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, model.FileUnlinkEventType, "unlink.file.path", "/etc/conf.d/abc"); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(model.SECLConstants, nil, enabled, nil, model.SECLLegacyAttributes, &log.PatternLogger{}))
	addRuleExpr(t, rs, `unlink.file.path =~ "/etc/conf.d/ab*"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, model.FileUnlinkEventType, "unlink.file.path", "/etc/conf.d/abc"); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(model.SECLConstants, nil, enabled, nil, model.SECLLegacyAttributes, &log.PatternLogger{}))
	addRuleExpr(t, rs, `unlink.file.path =~ "*/conf.d/ab*"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, model.FileUnlinkEventType, "unlink.file.path", "/etc/conf.d/abc"); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(model.SECLConstants, nil, enabled, nil, model.SECLLegacyAttributes, &log.PatternLogger{}))
	addRuleExpr(t, rs, `unlink.file.path =~ "*/conf.d"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, model.FileUnlinkEventType, "unlink.file.path", "/etc/conf.d/abc"); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(model.SECLConstants, nil, enabled, nil, model.SECLLegacyAttributes, &log.PatternLogger{}))
	addRuleExpr(t, rs, `unlink.file.path =~ "/etc/*"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, model.FileUnlinkEventType, "unlink.file.path", "/etc/cron.d/log"); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(model.SECLConstants, nil, enabled, nil, model.SECLLegacyAttributes, &log.PatternLogger{}))
	addRuleExpr(t, rs, `open.file.path == "/tmp/passwd"`, `open.file.path == "/tmp/secret"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, model.FileOpenEventType, "open.file.path", "/tmp/runc"); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(model.SECLConstants, nil, enabled, nil, model.SECLLegacyAttributes, &log.PatternLogger{}))
	addRuleExpr(t, rs, `open.file.path =~ "/run/secrets/kubernetes.io/serviceaccount/*/token"`, `open.file.path == "/etc/secret"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, model.FileOpenEventType, "open.file.path", "/tmp/token"); !is {
		t.Error("should be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(model.SECLConstants, nil, enabled, nil, model.SECLLegacyAttributes, &log.PatternLogger{}))
	addRuleExpr(t, rs, `open.file.path =~ "*/token"`, `open.file.path == "/etc/secret"`)

	is, err := isParentPathDiscarder(rs, regexCache, model.FileOpenEventType, "open.file.path", "/tmp/token")
	if err != nil {
		t.Error(err)
	}
	if is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, rules.NewOptsWithParams(model.SECLConstants, nil, enabled, nil, model.SECLLegacyAttributes, &log.PatternLogger{}))
	addRuleExpr(t, rs, `open.file.path =~ "/tmp/dir/no-approver-*"`)

	if is, _ := isParentPathDiscarder(rs, regexCache, model.FileOpenEventType, "open.file.path", "/tmp/dir/a/test"); !is {
		t.Error("should be a parent discarder")
	}
}
