// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package selftests holds selftests related files
package selftests

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// OpenSelfTest defines an open self test
type OpenSelfTest struct {
	ruleID    eval.RuleID
	filename  string
	isSuccess bool
}

// GetRuleDefinition returns the rule
func (o *OpenSelfTest) GetRuleDefinition() *rules.RuleDefinition {
	o.ruleID = ruleIDPrefix + "_open"

	return &rules.RuleDefinition{
		ID:         o.ruleID,
		Expression: fmt.Sprintf(`open.file.path == "%s" && open.flags & O_CREAT > 0`, o.filename),
		Silent:     true,
	}
}

// GenerateEvent generate an event
func (o *OpenSelfTest) GenerateEvent(ctx context.Context) error {
	o.isSuccess = false

	// we need to use touch (or any other external program) as our PID is discarded by probes
	// so the events would not be generated
	cmd := exec.CommandContext(ctx, "touch", o.filename)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error running touch: %w", err)
	}

	return nil
}

// HandleEvent handles self test events
func (o *OpenSelfTest) HandleEvent(event selfTestEvent) {
	if event.Event == nil ||
		event.Event.BaseEventSerializer == nil ||
		event.Event.BaseEventSerializer.FileEventSerializer == nil {
		seclog.Errorf("Open SelfTest event received with nil Event or File fields")
		o.isSuccess = false
		return
	}

	// debug logs
	if event.RuleID == o.ruleID && o.filename != event.Event.BaseEventSerializer.FileEventSerializer.Path {
		seclog.Errorf("Open SelfTest event received with different filepaths: %s VS %s", o.filename, event.Event.BaseEventSerializer.FileEventSerializer.Path)
	}

	o.isSuccess = event.RuleID == o.ruleID && o.filename == event.Event.BaseEventSerializer.FileEventSerializer.Path
}

// IsSuccess return the state of the test
func (o *OpenSelfTest) IsSuccess() bool {
	return o.isSuccess
}
