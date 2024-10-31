// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

package tests

import (
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestSnapshotReplay(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDef := &rules.RuleDefinition{
		ID:         "test_rule_snapshot_replay",
		Expression: "exec.comm in [\"testsuite\"]",
	}

	gotEvent := atomic.NewBool(false)

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{ruleDef}, withStaticOpts(testOpts{
		snapshotRuleMatchHandler: func(testMod *testModule, e *model.Event, r *rules.Rule) {
			assertTriggeredRule(t, r, "test_rule_snapshot_replay")
			testMod.validateExecSchema(t, e)
			gotEvent.Store(true)
		},
	}))

	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	if _, err := exec.Command("echo", "hello", "world").CombinedOutput(); err != nil {
		t.Fatal(err)
	}

	assert.Eventually(t, func() bool { return gotEvent.Load() }, 10*time.Second, 100*time.Millisecond, "didn't get the event from snapshot")
}
