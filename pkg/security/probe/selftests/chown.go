// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package selftests

import (
	"fmt"
	"os/exec"
	"os/user"

	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ChownSelfTest defines a chown self test
type ChownSelfTest struct{}

// GetRuleDefinition returns the rule
func (o *ChownSelfTest) GetRuleDefinition(filename string) *rules.RuleDefinition {
	return &rules.RuleDefinition{
		ID:         fmt.Sprintf("%s_chown", ruleIDPrefix),
		Expression: fmt.Sprintf(`chown.file.path == "%s"`, filename),
	}
}

// GenerateEvent generate an event
func (o *ChownSelfTest) GenerateEvent(filename string) (EventPredicate, error) {
	// we need to use chown (or any other external program) as our PID is discarded by probes
	// so the events would not be generated
	currentUser, err := user.Current()
	if err != nil {
		log.Debugf("error retrieving uid: %v", err)
		return nil, err
	}

	cmd := exec.Command("chown", currentUser.Uid, filename)
	if err := cmd.Run(); err != nil {
		log.Debugf("error running chown: %v", err)
		return nil, err
	}

	return func(event selfTestEvent) bool {
		return event.Type == "chown" && event.Filepath == filename
	}, nil
}
