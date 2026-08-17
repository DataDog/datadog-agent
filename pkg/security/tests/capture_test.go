// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/cenkalti/backoff/v7"
	"github.com/oliveagle/jsonpath"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// TestActionCaptureInherited reproduces the SSM Run Command correlation scenario: the
// agent creates an orchestration directory named after the CloudTrail SendCommand
// CommandId, and a silent rule captures that id out of the path into a process scoped
// inherited variable. Every descendant of the process that touched the directory then
// carries the id, which is what lets the backend join the kernel activity to the API
// call that started it.
func TestActionCaptureInherited(t *testing.T) {
	SkipIfNotAvailable(t)

	// shaped like a real SSM CommandId, so that the value also exercises the scrubber
	// applied to serialized variables
	const commandID = "4a1b2c3d-5e6f-7a8b-9c0d-1e2f3a4b5c6d"

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "capture_ssm_command_id",
			Expression: `open.file.path == "{{.Root}}/document/orchestration/` + commandID + `/awsrunShellScript"`,
			// the extraction itself is benign, only the artifact it attaches matters
			Silent: true,
			Actions: []*rules.ActionDefinition{
				{
					Set: &rules.SetDefinition{
						Name:      "ssm_command_id",
						Scope:     "process",
						Inherited: true,
						Field:     "open.file.path",
						Capture:   "/orchestration/([^/]+)/",
					},
				},
			},
		},
		{
			// fires on a grandchild of the process that opened the orchestration file,
			// and only if the variable holds the command id alone rather than the whole
			// path it was captured from
			ID: "descendant_carries_command_id",
			Expression: `open.file.path == "{{.Root}}/descendant-check" && ` +
				`${process.ssm_command_id} == "` + commandID + `"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	documentDir, _, err := test.Path("document")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(documentDir)

	triggerFile := filepath.Join(documentDir, "orchestration", commandID, "awsrunShellScript")
	if err := os.MkdirAll(filepath.Dir(triggerFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(triggerFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	checkFile, _, err := test.Path("descendant-check")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(checkFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(checkFile)

	// the redirections are performed by the shells themselves, so the opens are
	// attributed to them rather than to a short lived helper process: the outer shell
	// gets the captured variable, the inner one has to inherit it
	script := fmt.Sprintf(`: < %s; sh -c ": < %s"`, triggerFile, checkFile)

	test.WaitSignalFromRule(t, func() error {
		return exec.CommandContext(context.Background(), "sh", "-c", script).Run()
	}, func(_ *model.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "descendant_carries_command_id")
	}, "descendant_carries_command_id")

	// the artifact is only useful to the backend if it survives serialization, and in
	// particular if the scrubber leaves it intact
	err = retry(t, func() error {
		msg := test.msgSender.getMsg("descendant_carries_command_id")
		if msg == nil {
			return errors.New("message not found")
		}

		jsonPathValidation(test, msg.Data, func(_ *testModule, obj interface{}) {
			value, err := jsonpath.JsonPathLookup(obj, `$.process.variables.ssm_command_id`)
			if err != nil {
				t.Errorf("captured variable should be present in the serialized event: %v", err)
				return
			}
			assert.Equal(t, commandID, value, "captured variable should survive serialization untouched")
		})

		return nil
	}, backoff.WithMaxTries(10))
	if err != nil {
		t.Error(err)
	}

	// the extraction itself must stay silent: a rule firing on every benign SSM
	// operation is exactly what attaching the artifact to real events avoids
	if msg := test.msgSender.getMsg("capture_ssm_command_id"); msg != nil {
		t.Error("the silent capture rule should not have sent an event")
	}
}
