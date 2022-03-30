// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package module

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/pkg/errors"
)

func getSelfTestRuleDefinitions(baseRuleName, targetFilePath string) []*rules.RuleDefinition {
	openRule := &rules.RuleDefinition{
		ID:         fmt.Sprintf("%s_open", baseRuleName),
		Expression: fmt.Sprintf(`open.file.path == "%s"`, targetFilePath),
	}
	chmodRule := &rules.RuleDefinition{
		ID:         fmt.Sprintf("%s_chmod", baseRuleName),
		Expression: fmt.Sprintf(`chmod.file.path == "%s"`, targetFilePath),
	}
	chownRule := &rules.RuleDefinition{
		ID:         fmt.Sprintf("%s_chown", baseRuleName),
		Expression: fmt.Sprintf(`chown.file.path == "%s"`, targetFilePath),
	}

	return []*rules.RuleDefinition{openRule, chmodRule, chownRule}
}

// SelfTester represents all the state needed to conduct rule injection test at startup
type SelfTester struct {
	waitingForEvent uint32 // atomic bool
	eventChan       chan selfTestEvent
	targetFilePath  string
	targetTempDir   string
	success         []string
	fails           []string
}

// NewSelfTester returns a new SelfTester, enabled or not
func NewSelfTester() *SelfTester {
	return &SelfTester{
		waitingForEvent: 0,
		eventChan:       make(chan selfTestEvent, 10),
		success:         nil,
		fails:           nil,
	}
}

// CreateTargetFileIfNeeded creates the needed target file for self test operations
func (t *SelfTester) CreateTargetFileIfNeeded() error {
	if t.targetFilePath != "" {
		return nil
	}

	// Create temp directory to put target file in
	tmpDir, err := os.MkdirTemp("", "datadog_agent_cws_self_test")
	if err != nil {
		return err
	}
	t.targetTempDir = tmpDir

	// Create target file
	targetFile, err := os.CreateTemp(tmpDir, "datadog_agent_cws_target_file")
	if err != nil {
		return err
	}
	t.targetFilePath = targetFile.Name()

	return targetFile.Close()
}

const (
	selfTestPolicyName     = "datadog-agent-cws-self-test-policy"
	selfTestPolicyFilename = "datadog_cws_self_test.policy"
	selfTestBaseRuleName   = "datadog_agent_cws_self_test_rule"
)

// GetSelfTestPolicy returns the additional policy containing self test rules
func (t *SelfTester) GetSelfTestPolicy() *rules.Policy {
	rds := getSelfTestRuleDefinitions(selfTestBaseRuleName, t.targetFilePath)
	p := &rules.Policy{
		Name:    selfTestPolicyName,
		Version: "1.0.0",
	}

	for _, rd := range rds {
		rd.Policy = p
	}

	p.Rules = rds
	return p
}

// AddSelfTestRulesToRuleSets adds self test rules to the rulesets
func (t *SelfTester) AddSelfTestRulesToRuleSets(ruleSet, approverRuleSet *rules.RuleSet) {
	selfTestPolicy := t.GetSelfTestPolicy()

	ruleSet.AddPolicyVersion(selfTestPolicyFilename, selfTestPolicy.Version)
	approverRuleSet.AddPolicyVersion(selfTestPolicyFilename, selfTestPolicy.Version)

	_, rules, merr := selfTestPolicy.GetValidMacroAndRules()
	if merr.ErrorOrNil() != nil {
		logMultiErrors("error while loading additional policies", merr)
	}

	if len(rules) != 0 {
		if merr := ruleSet.AddRules(rules); merr.ErrorOrNil() != nil {
			logMultiErrors("error while loading additional policies", merr)
		}

		if merr := approverRuleSet.AddRules(rules); merr.ErrorOrNil() != nil {
			logMultiErrors("error while loading additional policies", merr)
		}
	}
}

// RunSelfTest runs the self test and return the result
func (t *SelfTester) RunSelfTest() ([]string, []string, error) {
	if err := t.BeginWaitingForEvent(); err != nil {
		return nil, nil, errors.Wrap(err, "failed to run self test")
	}
	defer t.EndWaitingForEvent()

	// launch the self tests
	var lastErr error
	var success []string
	var fails []string
	for _, fn := range SelfTestFunctions {
		if err := fn.fn(t); err != nil {
			lastErr = err
			fails = append(fails, fn.id)
		} else {
			success = append(success, fn.id)
		}
	}

	// save the results for get status command
	t.success = success
	t.fails = fails

	return success, fails, lastErr
}

// Cleanup removes temp directories and files used by the self tester
func (t *SelfTester) Cleanup() error {
	if t.targetTempDir != "" {
		return os.RemoveAll(t.targetTempDir)
	}
	return nil
}

// BeginWaitingForEvent passes the tester in the waiting for event state
func (t *SelfTester) BeginWaitingForEvent() error {
	if atomic.SwapUint32(&t.waitingForEvent, 1) != 0 {
		return errors.New("a self test is already running")
	}
	return nil
}

// EndWaitingForEvent exits the waiting for event state
func (t *SelfTester) EndWaitingForEvent() {
	atomic.StoreUint32(&t.waitingForEvent, 0)
}

type selfTestEvent struct {
	Type     string
	Filepath string
}

// isExpectedEvent sends an event to the tester
func (t *SelfTester) isExpectedEvent(rule *rules.Rule, event eval.Event) bool {
	if atomic.LoadUint32(&t.waitingForEvent) != 0 && rule.Definition.Policy.Name == selfTestPolicyName {
		ev, ok := event.(*probe.Event)
		if !ok {
			return true
		}

		s := probe.NewEventSerializer(ev)
		if s == nil || s.FileEventSerializer == nil {
			return true
		}

		selfTestEvent := selfTestEvent{
			Type:     event.GetType(),
			Filepath: s.FileEventSerializer.Path,
		}
		t.eventChan <- selfTestEvent
		return true
	}
	return false
}

func (t *SelfTester) expectEvent(predicate func(selfTestEvent) bool) error {
	timer := time.After(3 * time.Second)
	for {
		select {
		case event := <-t.eventChan:
			if predicate(event) {
				return nil
			}
		case <-timer:
			return errors.New("failed to receive expected event")
		}
	}
}

func selfTestOpen(t *SelfTester) error {
	// we need to use touch (or any other external program) as our PID is discarded by probes
	// so the events would not be generated
	cmd := exec.Command("touch", t.targetFilePath)
	if err := cmd.Run(); err != nil {
		log.Debugf("error running touch: %v", err)
		return err
	}

	return t.expectEvent(func(event selfTestEvent) bool {
		return event.Type == "open" && event.Filepath == t.targetFilePath
	})
}

func selfTestChmod(t *SelfTester) error {
	// we need to use chmod (or any other external program) as our PID is discarded by probes
	// so the events would not be generated
	cmd := exec.Command("chmod", "777", t.targetFilePath)
	if err := cmd.Run(); err != nil {
		log.Debugf("error running chmod: %v", err)
		return err
	}

	return t.expectEvent(func(event selfTestEvent) bool {
		return event.Type == "chmod" && event.Filepath == t.targetFilePath
	})
}

func selfTestChown(t *SelfTester) error {
	// we need to use chown (or any other external program) as our PID is discarded by probes
	// so the events would not be generated
	currentUser, err := user.Current()
	if err != nil {
		log.Debugf("error retrieving uid: %v", err)
		return err
	}

	cmd := exec.Command("chown", currentUser.Uid, t.targetFilePath)
	if err := cmd.Run(); err != nil {
		log.Debugf("error running chown: %v", err)
		return err
	}

	return t.expectEvent(func(event selfTestEvent) bool {
		return event.Type == "chown" && event.Filepath == t.targetFilePath
	})
}

// SelfTestFunction represent one self test, with its ID and func
type SelfTestFunction struct {
	id string
	fn func(*SelfTester) error
}

// SelfTestFunctions slice of self test functions representing each individual file test
var SelfTestFunctions = []SelfTestFunction{
	{"selfTestOpen", selfTestOpen},
	{"selfTestChmod", selfTestChmod},
	{"selfTestChown", selfTestChown},
}
