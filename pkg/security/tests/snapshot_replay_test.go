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
	"log"
	"os/exec"
	"testing"
)

func TestSnapshotReplay(t *testing.T) {
	ncExec := which(t, "nc")
	cmd := exec.Command(ncExec, "-l", "4242")

	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	defer cmd.Process.Kill()

	ruleDef := &rules.RuleDefinition{
		ID:         "test_rule_nc",
		Expression: fmt.Sprintf(`exec.comm in ["socat", "dig", "nslookup", "host", ~"netcat*", ~"nc*", "ncat"] `),
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
			assertTriggeredRule(t, rule, "test_rule_nc")
			test.validateExecSchema(t, event)
		})
	})

}
