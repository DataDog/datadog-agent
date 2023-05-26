// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

func TestIsParentDiscarder(t *testing.T) {
	id := newInodeDiscarders(nil, nil)

	enabled := map[eval.EventType]bool{"*": true}

	var evalOpts eval.Opts
	evalOpts.
		WithConstants(model.SECLConstants).
		WithLegacyFields(model.SECLLegacyFields).
		WithVariables(model.SECLVariables)

	var opts rules.Opts
	opts.
		WithEventTypeEnabled(enabled).
		WithLogger(seclog.DefaultLogger)

	rs := rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path =~ "/var/log/*" && unlink.file.path != "/var/log/datadog/system-probe.log"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/log/datadog/system-probe.log", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path =~ "/var/log/*" && unlink.file.path != "/var/log/datadog/system-probe.log"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/lib/datadog/system-probe.sock", 1); !is {
		t.Error("should be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path == "/var/log/datadog/system-probe.log"`, `unlink.file.name == "datadog"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/log/datadog/datadog-agent.log", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path =~ "/var/log/*" && unlink.file.name =~ ".*"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/lib/.runc/1234", 1); is {
		t.Error("shouldn't be able to find a parent discarder, due to partial evaluation: true && unlink.file.name =~ '.*'")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path == "/etc/conf.d/httpd.conf" || unlink.file.name == "conf.d"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/etc/conf.d/nginx.conf", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path == "/etc/conf.d/httpd.conf" || unlink.file.name == "sys.d"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/etc/sys.d/nginx.conf", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.name == "conf.d"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/etc/conf.d/nginx.conf", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	// field that doesn't exists shouldn't return any discarders
	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `rename.file.path == "/etc/conf.d/abc"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileRenameEventType, "rename.file.path", "/etc/conf.d/nginx.conf", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `rename.file.path == "/etc/conf.d/abc"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileRenameEventType, "rename.file.path", "/etc/nginx/nginx.conf", 1); !is {
		t.Error("should be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path =~ "/etc/conf.d/*"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/etc/sys.d/nginx.conf", 1); !is {
		t.Error("should be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path =~ "*/conf.*"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/etc/conf.d/abc", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path =~ "/etc/conf.d/ab*"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/etc/conf.d/abc", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path =~ "*/conf.d/ab*"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/etc/conf.d/abc", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path =~ "*/conf.d"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/etc/conf.d/abc", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path =~ "/etc/*"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/etc/cron.d/log", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `open.file.path == "/tmp/passwd"`, `open.file.path == "/tmp/secret"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileOpenEventType, "open.file.path", "/tmp/runc", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `open.file.path =~ "/run/secrets/kubernetes.io/serviceaccount/*/token"`, `open.file.path == "/etc/secret"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileOpenEventType, "open.file.path", "/tmp/token", 1); !is {
		t.Error("should be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `open.file.path =~ "*/token"`, `open.file.path == "/etc/secret"`)

	is, err := id.isParentPathDiscarder(rs, model.FileOpenEventType, "open.file.path", "/tmp/token", 1)
	if err != nil {
		t.Error(err)
	}
	if is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `open.file.path =~ "/tmp/dir/no-approver-*"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileOpenEventType, "open.file.path", "/tmp/dir/a/test", 1); !is {
		t.Error("should be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `open.file.path =~ "/"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileOpenEventType, "open.file.path", "/tmp", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `open.file.path =~ "*/conf.d/aaa"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileOpenEventType, "open.file.path", "/tmp/dir/bbb", 1); !is {
		t.Error("should be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `open.file.path =~ "/etc/**"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileOpenEventType, "open.file.path", "/etc/conf.d/dir/aaa", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `open.file.path == "/proc/${process.pid}/maps"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileOpenEventType, "open.file.path", "/proc/1/maps", 1); is {
		t.Error("shouldn't be a parent discarder")
	}

	// test basename conflict, a basename based rule matches the parent discarder
	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `open.file.path =~ "/var/log/datadog/**" && open.file.name == "token"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileOpenEventType, "open.file.path", "/tmp/test1/test2", 1); !is {
		t.Error("should be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `open.file.path =~ "/var/log/datadog/**" && open.file.name == "test1"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileOpenEventType, "open.file.path", "/tmp/test1/test2", 1); is {
		t.Error("shouldn't be a parent discarder")
	}
}

func TestIsGrandParentDiscarder(t *testing.T) {
	id := newInodeDiscarders(nil, nil)

	enabled := map[eval.EventType]bool{"*": true}

	var evalOpts eval.Opts
	evalOpts.
		WithConstants(model.SECLConstants).
		WithLegacyFields(model.SECLLegacyFields)

	var opts rules.Opts
	opts.
		WithEventTypeEnabled(enabled).
		WithLogger(seclog.DefaultLogger)

	rs := rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path == "/var/lib/datadog/system-probe.cache"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/run/datadog/system-probe.pid", 2); !is {
		t.Error("should be a grand parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path =~ "/tmp/test"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/lib", 2); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path == "/var/run/datadog/system-probe.pid"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/run/pids/system-probe.pid", 2); is {
		t.Error("shouldn't be a grand parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path =~ "/var/lib/datadog/system-probe.cache"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/run/datadog/system-probe.pid", 2); !is {
		t.Error("should be a grand parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path =~ "/var/run/datadog/system-probe.pid"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/run/pids/system-probe.pid", 2); is {
		t.Error("shouldn't be a grand parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path =~ "*/run/datadog/system-probe.pid"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/run/datadog/system-probe.pid", 2); is {
		t.Error("shouldn't be a grand parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path =~ "*/run/datadog/system-probe.pid"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/lib/datadog/system-probe.pid", 2); !is {
		t.Error("should be a grand parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path =~ "/var/*/datadog/system-probe.pid"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/run/datadog/system-probe.pid", 2); is {
		t.Error("shouldn't be a grand parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path =~ "/var/lib/datadog/system-probe.pid"`, `unlink.file.name =~ "run"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/run/datadog/system-probe.pid", 2); is {
		t.Error("shouldn't be a grand parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path =~ "/var/*"`, `unlink.file.name =~ "run"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/var/run/datadog/system-probe.pid", 2); is {
		t.Error("shouldn't be a grand parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `open.file.path =~ "/tmp/dir/*"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileOpenEventType, "open.file.path", "/tmp/dir/a/test", 2); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.name == "dir"`) // + variants

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/tmp/dir/a/test", 2); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path == "/tmp/dir/a"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/tmp", 2); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path == "/tmp"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/tmp/dir/a", 2); is {
		t.Error("shouldn't be a parent discarder")
	}

	rs = rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path == "/tmp"`)

	if is, _ := id.isParentPathDiscarder(rs, model.FileUnlinkEventType, "unlink.file.path", "/tmp", 2); is {
		t.Error("shouldn't be a parent discarder")
	}
}

type testEventListener struct {
	fields map[eval.Field]int
}

func (l *testEventListener) RuleMatch(rule *rules.Rule, event eval.Event) {}

func (l *testEventListener) EventDiscarderFound(rs *rules.RuleSet, event eval.Event, field eval.Field, eventType eval.EventType) {
	if l.fields == nil {
		l.fields = make(map[eval.Field]int)
	}
	l.fields[field]++
}

func TestIsDiscarderOverride(t *testing.T) {
	enabled := map[eval.EventType]bool{"*": true}

	var evalOpts eval.Opts
	evalOpts.
		WithConstants(model.SECLConstants).
		WithLegacyFields(model.SECLLegacyFields)

	var opts rules.Opts
	opts.
		WithEventTypeEnabled(enabled).
		WithLogger(seclog.DefaultLogger)

	var listener testEventListener

	rs := rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	rs.AddListener(&listener)
	kfilters.AddRuleExpr(t, rs, `unlink.file.path == "/var/log/httpd" && process.file.path == "/bin/touch"`)

	event := rs.NewEvent().(*model.Event)
	event.Init()

	event.Type = uint32(model.FileUnlinkEventType)
	event.SetFieldValue("unlink.file.path", "/var/log/httpd")
	event.SetFieldValue("process.file.path", "/bin/touch")

	if !rs.Evaluate(event) {
		rs.EvaluateDiscarders(event)
	}

	if listener.fields["process.file.path"] > 0 {
		t.Error("shouldn't get a discarder")
	}

	event.SetFieldValue("process.file.path", "/bin/cat")

	if !rs.Evaluate(event) {
		rs.EvaluateDiscarders(event)
	}

	if listener.fields["process.file.path"] == 0 {
		t.Error("should get a discarder")
	}
}

func BenchmarkParentDiscarder(b *testing.B) {
	id := newInodeDiscarders(nil, nil)

	enabled := map[eval.EventType]bool{"*": true}

	var evalOpts eval.Opts
	evalOpts.
		WithConstants(model.SECLConstants).
		WithLegacyFields(model.SECLLegacyFields)

	var opts rules.Opts
	opts.
		WithEventTypeEnabled(enabled).
		WithLogger(seclog.DefaultLogger)

	rs := rules.NewRuleSet(&model.Model{}, model.NewDefaultEvent, &opts, &evalOpts)
	kfilters.AddRuleExpr(b, rs, `unlink.file.path =~ "/var/log/*" && unlink.file.path != "/var/log/datadog/system-probe.log"`)

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
