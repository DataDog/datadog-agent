// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests
// +build functionaltests

package tests

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"testing"
)

func TestSnapshotReplay(t *testing.T) {
	ruleDef := &rules.RuleDefinition{
		ID:         "test_rule_snapshot_replay",
		Expression: fmt.Sprintf(`exec.comm in ["testsuite"] `),
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{ruleDef}, testOpts{})

	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("snapshot-replay", func(t *testing.T) {
		// Check that the process is present in the process resolver's entrycache
		test.WaitSignal(t, func() error {
			go test.probe.PlaySnapshot()
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_snapshot_replay")
			test.validateExecSchema(t, event)
		})
	})

}
