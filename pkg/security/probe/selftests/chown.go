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
	"os/user"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ChownSelfTest defines a chown self test
type ChownSelfTest struct {
	ruleID    eval.RuleID
	filename  string
	isSuccess bool
}

// GetRuleDefinition returns the rule
func (o *ChownSelfTest) GetRuleDefinition() *rules.RuleDefinition {
	o.ruleID = fmt.Sprintf("%s_chown", ruleIDPrefix)

	return &rules.RuleDefinition{
		ID:         o.ruleID,
		Expression: fmt.Sprintf(`chown.file.path == "%s"`, o.filename),
		Silent:     true,
	}
}

// GenerateEvent generate an event
func (o *ChownSelfTest) GenerateEvent() error {
	o.isSuccess = false

	// we need to use chown (or any other external program) as our PID is discarded by probes
	// so the events would not be generated
	currentUser, err := user.Current()
	if err != nil {
		log.Debugf("error retrieving uid: %v", err)
		return err
	}

	cmd := exec.Command("chown", currentUser.Uid, o.filename)
	if err := cmd.Run(); err != nil {
		log.Debugf("error running chown: %v", err)
		return err
	}

	return nil
}

// HandleEvent handles self test events
func (o *ChownSelfTest) HandleEvent(event selfTestEvent) {
	o.isSuccess = event.RuleID == o.ruleID
}

// IsSuccess return the state of the test
func (o *ChownSelfTest) IsSuccess() bool {
	return o.isSuccess
}
