// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package bundled

import (
	"path"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// TestNeedRefreshSBOMRule checks that the rule marking a process for an SBOM
// refresh matches the writes of a package database, and leaves out the files
// every rpm process opens read-write, a query included, so a query keeps the
// SBOM as it is.
func TestNeedRefreshSBOMRule(t *testing.T) {
	var expr string
	for _, def := range newBundledPolicyRules(&config.RuntimeSecurityConfig{SBOMResolverEnabled: true}) {
		if def.ID == NeedRefreshSBOMRuleID {
			expr = def.Expression
		}
	}
	if expr == "" {
		t.Fatalf("no %s rule", NeedRefreshSBOMRuleID)
	}

	ruleOpts, evalOpts := rules.NewBothOpts(map[eval.EventType]bool{"*": true})
	rs := rules.NewRuleSet(&model.Model{}, func() eval.Event { return model.NewFakeEvent() }, ruleOpts, evalOpts)
	rules.AddTestRuleExpr(t, rs, expr)

	tests := []struct {
		path  string
		flags int
		want  bool
	}{
		{"/usr/lib/sysimage/rpm/rpmdb.sqlite", syscall.O_RDWR | syscall.O_CREAT, true},
		{"/usr/lib/sysimage/rpm/.rpm.lock", syscall.O_RDWR | syscall.O_CREAT, true},
		{"/var/lib/rpm/Packages", syscall.O_RDWR | syscall.O_CREAT, true},
		{"/usr/lib/sysimage/rpm/Packages.db", syscall.O_RDWR, true},
		{"/var/lib/dpkg/lock", syscall.O_RDWR | syscall.O_CREAT, true},
		{"/lib/apk/db/installed", syscall.O_WRONLY | syscall.O_CREAT, true},
		{"/usr/lib/sysimage/rpm/rpmdb.sqlite", syscall.O_RDONLY, false},
		{"/usr/lib/sysimage/rpm/rpmdb.sqlite-shm", syscall.O_RDWR | syscall.O_CREAT, false},
		{"/usr/lib/sysimage/rpm/rpmdb.sqlite-wal", syscall.O_RDWR | syscall.O_CREAT, false},
		{"/var/lib/rpm/__db.001", syscall.O_RDWR | syscall.O_CREAT, false},
		{"/var/lib/rpm/.dbenv.lock", syscall.O_RDWR | syscall.O_CREAT, false},
		{"/usr/lib/sysimage/rpm/Index.db", syscall.O_RDWR, false},
	}

	for _, tt := range tests {
		ev := model.NewFakeEvent()
		ev.Type = uint32(model.FileOpenEventType)
		ev.SetFieldValue("open.file.path", tt.path)
		ev.SetFieldValue("open.file.name", path.Base(tt.path))
		ev.SetFieldValue("open.flags", tt.flags)

		if got := rs.Evaluate(ev); got != tt.want {
			t.Errorf("open(%q, %#o) matches = %v, want %v", tt.path, tt.flags, got, tt.want)
		}
	}
}
