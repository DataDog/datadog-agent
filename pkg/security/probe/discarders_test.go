// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"fmt"
	"testing"
	"time"

	seclog "github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func addRuleExpr(t testing.TB, rs *rules.RuleSet, exprs ...string) {
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
	id, _ := newInodeDiscarders(nil, nil, nil, nil)

	enabled := map[eval.EventType]bool{"*": true}

	var evalOpts eval.Opts
	evalOpts.
		WithConstants(model.SECLConstants).
		WithLegacyFields(model.SECLLegacyFields)

	var opts rules.Opts
	opts.
		WithEventTypeEnabled(enabled).
		WithLogger(&seclog.PatternLogger{})

	rs := rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path =~ "/var/log/*" && unlink.file.path != "/var/log/datadog/system-probe.log"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/log/datadog/system-probe.log", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path =~ "/var/log/*" && unlink.file.path != "/var/log/datadog/system-probe.log"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/lib/datadog/system-probe.sock", 1); !is {
		t.Error("should be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path == "/var/log/datadog/system-probe.log"`, `unlink.file.name == "datadog"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/log/datadog/datadog-agent.log", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path =~ "/var/log/*" && unlink.file.name =~ ".*"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/lib/.runc/1234", 1); is {
		t.Error("shouldn't be able to find a parent discarder, due to partial evaluation: true && unlink.file.name =~ '.*'")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path == "/etc/conf.d/httpd.conf" || unlink.file.name == "conf.d"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/etc/conf.d/nginx.conf", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path == "/etc/conf.d/httpd.conf" || unlink.file.name == "sys.d"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/etc/sys.d/nginx.conf", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.name == "conf.d"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/etc/conf.d/nginx.conf", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	// field that doesn't exists shouldn't return any discarders
	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `rename.file.path == "/etc/conf.d/abc"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileRenameEventType, "rename.file.path", "/etc/conf.d/nginx.conf", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `rename.file.path == "/etc/conf.d/abc"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileRenameEventType, "rename.file.path", "/etc/nginx/nginx.conf", 1); !is {
		t.Error("should be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path =~ "/etc/conf.d/*"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/etc/sys.d/nginx.conf", 1); !is {
		t.Error("should be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path =~ "*/conf.*"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/etc/conf.d/abc", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path =~ "/etc/conf.d/ab*"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/etc/conf.d/abc", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path =~ "*/conf.d/ab*"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/etc/conf.d/abc", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path =~ "*/conf.d"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/etc/conf.d/abc", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path =~ "/etc/*"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/etc/cron.d/log", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `open.file.path == "/tmp/passwd"`, `open.file.path == "/tmp/secret"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileOpenEventType, "open.file.path", "/tmp/runc", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `open.file.path =~ "/run/secrets/kubernetes.io/serviceaccount/*/token"`, `open.file.path == "/etc/secret"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileOpenEventType, "open.file.path", "/tmp/token", 1); !is {
		t.Error("should be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `open.file.path =~ "*/token"`, `open.file.path == "/etc/secret"`)

	is, err := id.isParentPathDiscarder(rs, model.FileOpenEventType, "open.file.path", "/tmp/token", 1)
	if err != nil {
		t.Error(err)
	}
	if is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `open.file.path =~ "/tmp/dir/no-approver-*"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileOpenEventType, "open.file.path", "/tmp/dir/a/test", 1); !is {
		t.Error("should be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `open.file.path =~ "/"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileOpenEventType, "open.file.path", "/tmp", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `open.file.path =~ "*/conf.d/aaa"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileOpenEventType, "open.file.path", "/tmp/dir/bbb", 1); !is {
		t.Error("should be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `open.file.path =~ "/etc/**"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileOpenEventType, "open.file.path", "/etc/conf.d/dir/aaa", 1); is {
		t.Error("shouldn't be a parent discarder")
	}
}

func TestIsGrandParentDiscarder(t *testing.T) {
	id, _ := newInodeDiscarders(nil, nil, nil, nil)

	enabled := map[eval.EventType]bool{"*": true}

	var evalOpts eval.Opts
	evalOpts.
		WithConstants(model.SECLConstants).
		WithLegacyFields(model.SECLLegacyFields)

	var opts rules.Opts
	opts.
		WithEventTypeEnabled(enabled).
		WithLogger(&seclog.PatternLogger{})

	rs := rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path == "/var/lib/datadog/system-probe.cache"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/run/datadog/system-probe.pid", 2); !is {
		t.Error("should be a grand parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path =~ "/tmp/test"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/lib", 2); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path == "/var/run/datadog/system-probe.pid"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/run/pids/system-probe.pid", 2); is {
		t.Error("shouldn't be a grand parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path =~ "/var/lib/datadog/system-probe.cache"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/run/datadog/system-probe.pid", 2); !is {
		t.Error("should be a grand parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path =~ "/var/run/datadog/system-probe.pid"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/run/pids/system-probe.pid", 2); is {
		t.Error("shouldn't be a grand parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path =~ "*/run/datadog/system-probe.pid"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/run/datadog/system-probe.pid", 2); is {
		t.Error("shouldn't be a grand parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path =~ "*/run/datadog/system-probe.pid"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/lib/datadog/system-probe.pid", 2); !is {
		t.Error("should be a grand parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path =~ "/var/*/datadog/system-probe.pid"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/run/datadog/system-probe.pid", 2); is {
		t.Error("shouldn't be a grand parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path =~ "/var/lib/datadog/system-probe.pid"`, `unlink.file.name =~ "run"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/run/datadog/system-probe.pid", 2); is {
		t.Error("shouldn't be a grand parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path =~ "/var/*"`, `unlink.file.name =~ "run"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/run/datadog/system-probe.pid", 2); is {
		t.Error("shouldn't be a grand parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `open.file.path =~ "/tmp/dir/*"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileOpenEventType, "open.file.path", "/tmp/dir/a/test", 2); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.name == "dir"`) // + variants

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/tmp/dir/a/test", 2); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path == "/tmp/dir/a"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/tmp", 2); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path == "/tmp"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/tmp/dir/a", 2); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(t, rs, `unlink.file.path == "/tmp"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/tmp", 2); is {
		t.Error("shouldn't be a parent discarder")
	}
}

func BenchmarkParentDiscarder(b *testing.B) {
	id, _ := newInodeDiscarders(nil, nil, nil, nil)

	enabled := map[eval.EventType]bool{"*": true}

	var evalOpts eval.Opts
	evalOpts.
		WithConstants(model.SECLConstants).
		WithLegacyFields(model.SECLLegacyFields)

	var opts rules.Opts
	opts.
		WithEventTypeEnabled(enabled).
		WithLogger(&seclog.PatternLogger{})

	rs := rules.NewRuleSet(&Model{}, func() eval.Event { return &Event{} }, &opts, &evalOpts, &eval.MacroStore{})
	addRuleExpr(b, rs, `unlink.file.path =~ "/var/log/*" && unlink.file.path != "/var/log/datadog/system-probe.log"`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/log/datadog/system-probe.log", 1)
	}
}

func TestIsRecentlyAdded(t *testing.T) {
	var id inodeDiscarders

	if id.isRecentlyAdded(1, 2, uint64(time.Now().UnixNano())) {
		t.Error("shouldn't be marked as added")
	}
	id.recentlyAdded(1, 2, uint64(time.Now().UnixNano()))

	if !id.isRecentlyAdded(1, 2, uint64(time.Now().UnixNano())) {
		t.Error("should be marked as added")
	}

	time.Sleep(time.Duration(recentlyAddedTimeout))

	if id.isRecentlyAdded(1, 2, uint64(time.Now().UnixNano())) {
		t.Error("shouldn't be marked as added")
	}
}

func TestIsRecentlyAddedCollision(t *testing.T) {
	var id inodeDiscarders

	if recentlyAddedIndex(0, 2) != recentlyAddedIndex(0, 2+maxRecentlyAddedCacheSize) {
		t.Error("unable to test without collision")
	}

	if id.isRecentlyAdded(0, 2, uint64(time.Now().UnixNano())) {
		t.Error("shouldn't be marked as added")
	}
	id.recentlyAdded(0, 2, uint64(time.Now().UnixNano()))

	if !id.isRecentlyAdded(0, 2, uint64(time.Now().UnixNano())) {
		t.Error("should be marked as added")
	}

	if id.isRecentlyAdded(0, 2+maxRecentlyAddedCacheSize, uint64(time.Now().UnixNano())) {
		t.Error("shouldn't be marked as added")
	}
}

func TestIsRecentlyAddedOverflow(t *testing.T) {
	var id inodeDiscarders

	if id.isRecentlyAdded(0, 2, 2) {
		t.Error("shouldn't be marked as added")
	}
	id.recentlyAdded(0, 2, 2)

	if !id.isRecentlyAdded(0, 2, 1) {
		t.Error("should be marked as added")
	}
}
