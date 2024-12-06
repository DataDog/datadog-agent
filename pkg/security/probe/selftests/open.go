// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package selftests holds selftests related files
package selftests

import (
	"fmt"
	"os/exec"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// OpenSelfTest defines an open self test
type OpenSelfTest struct {
	ruleID    eval.RuleID
	filename  string
	isSuccess bool
}

// GetRuleDefinition returns the rule
func (o *OpenSelfTest) GetRuleDefinition() *rules.RuleDefinition {
	o.ruleID = fmt.Sprintf("%s_open", ruleIDPrefix)

	return &rules.RuleDefinition{
		ID:         o.ruleID,
		Expression: fmt.Sprintf(`open.file.path == "%s" && open.flags & O_CREAT > 0`, o.filename),
		Silent:     true,
	}
}

// GenerateEvent generate an event
func (o *OpenSelfTest) GenerateEvent() error {
	o.isSuccess = false

	// we need to use touch (or any other external program) as our PID is discarded by probes
	// so the events would not be generated
	cmd := exec.Command("touch", o.filename)
	if err := cmd.Run(); err != nil {
		log.Debugf("error running touch: %v", err)
		return err
	}

	return nil
}

// HandleEvent handles self test events
func (o *OpenSelfTest) HandleEvent(event selfTestEvent) {
	o.isSuccess = event.RuleID == o.ruleID
}

// IsSuccess return the state of the test
func (o *OpenSelfTest) IsSuccess() bool {
	return o.isSuccess
}
