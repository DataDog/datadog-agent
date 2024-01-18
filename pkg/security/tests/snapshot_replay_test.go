// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestSnapshotReplay(t *testing.T) {
	ruleDef := &rules.RuleDefinition{
		ID:         "test_rule_snapshot_replay",
		Expression: "exec.comm in [\"testsuite\"]",
	}

	var gotEvent bool

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{ruleDef}, withStaticOpts(testOpts{
		snapshotRuleMatchHandler: func(testMod *testModule, e *model.Event, r *rules.Rule) {
			assertTriggeredRule(t, r, "test_rule_snapshot_replay")
			testMod.validateExecSchema(t, e)
			gotEvent = true
		},
	}))

	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	assert.True(t, gotEvent, "didn't get the event from snapshot")
}
