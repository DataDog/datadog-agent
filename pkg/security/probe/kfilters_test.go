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

	if is, _ := isParentPathDiscarder(rs, "unlink", "/var/log/datadog/system-probe.log"); is {
		t.Fatal("shouldn't be a parent discarder")
	}
}
