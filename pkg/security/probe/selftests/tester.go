// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package selftests

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/go-multierror"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	policySource  = "self-test"
	policyVersion = "1.0.0"
	policyName    = "datadog-agent-cws-self-test-policy"
	ruleIDPrefix  = "datadog_agent_cws_self_test_rule"
)

// EventPredicate defines a self test event validation predicate
type EventPredicate func(event selfTestEvent) bool

// FileSelfTest represent one self test, with its ID and func
type FileSelfTest interface {
	GetRuleDefinition(filename string) *rules.RuleDefinition
	GenerateEvent(filename string) (EventPredicate, error)
}

// FileSelfTests slice of self test functions representing each individual file test
var FileSelfTests = []FileSelfTest{
	&OpenSelfTest{},
	&ChmodSelfTest{},
	&ChownSelfTest{},
}

// SelfTester represents all the state needed to conduct rule injection test at startup
type SelfTester struct {
	waitingForEvent *atomic.Bool
	eventChan       chan selfTestEvent
	success         []string
	fails           []string
	lastTimestamp   time.Time

	// file tests
	targetFilePath string
	targetTempDir  string
}

var _ rules.PolicyProvider = (*SelfTester)(nil)

// NewSelfTester returns a new SelfTester, enabled or not
func NewSelfTester() (*SelfTester, error) {
	s := &SelfTester{
		waitingForEvent: atomic.NewBool(false),
		eventChan:       make(chan selfTestEvent, 10),
	}

	if err := s.createTargetFile(); err != nil {
		return nil, err
	}

	return s, nil
}

// GetStatus returns the result of the last performed self tests
func (t *SelfTester) GetStatus() *api.SelfTestsStatus {
	return &api.SelfTestsStatus{
		LastTimestamp: t.lastTimestamp.Format(time.RFC822),
		Success:       t.success,
		Fails:         t.fails,
	}
}

// LoadPolicies implements the PolicyProvider interface
func (t *SelfTester) LoadPolicies(macroFilters []rules.MacroFilter, ruleFilters []rules.RuleFilter) ([]*rules.Policy, *multierror.Error) {
	p := &rules.Policy{
		Name:    policyName,
		Source:  policySource,
		Version: policyVersion,
	}

	for _, selftest := range FileSelfTests {
		p.AddRule(selftest.GetRuleDefinition(t.targetFilePath))
	}

	return []*rules.Policy{p}, nil
}

// SetOnNewPoliciesReadyCb implements the PolicyProvider interface
func (t *SelfTester) SetOnNewPoliciesReadyCb(cb func()) {
}

func (t *SelfTester) createTargetFile() error {
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

// RunSelfTest runs the self test and return the result
func (t *SelfTester) RunSelfTest() ([]string, []string, error) {
	if err := t.BeginWaitingForEvent(); err != nil {
		return nil, nil, fmt.Errorf("failed to run self test: %w", err)
	}
	defer t.EndWaitingForEvent()

	t.lastTimestamp = time.Now()

	// launch the self tests
	var success []string
	var fails []string
	for _, selftest := range FileSelfTests {
		def := selftest.GetRuleDefinition(t.targetFilePath)

		predicate, err := selftest.GenerateEvent(t.targetFilePath)
		if err != nil {
			fails = append(fails, def.ID)
			log.Errorf("Self test failed: %s", def.ID)
			continue
		}

		if err = t.expectEvent(predicate); err != nil {
			fails = append(fails, def.ID)
			log.Errorf("Self test failed: %s", def.ID)
		} else {
			success = append(success, def.ID)
		}
	}

	// save the results for get status command
	t.success = success
	t.fails = fails

	return success, fails, nil
}

// Start starts the self tester policy provider
func (t *SelfTester) Start() {}

// Close removes temp directories and files used by the self tester
func (t *SelfTester) Close() error {
	if t.targetTempDir != "" {
		err := os.RemoveAll(t.targetTempDir)
		t.targetTempDir = ""
		return err
	}
	return nil
}

// BeginWaitingForEvent passes the tester in the waiting for event state
func (t *SelfTester) BeginWaitingForEvent() error {
	if t.waitingForEvent.Swap(true) {
		return errors.New("a self test is already running")
	}
	return nil
}

// EndWaitingForEvent exits the waiting for event state
func (t *SelfTester) EndWaitingForEvent() {
	t.waitingForEvent.Store(false)
}

type selfTestEvent struct {
	Type     string
	Filepath string
}

// IsExpectedEvent sends an event to the tester
func (t *SelfTester) IsExpectedEvent(rule *rules.Rule, event eval.Event, p *probe.Probe) bool {
	if t.waitingForEvent.Load() && rule.Definition.Policy.Source == policySource {
		ev, ok := event.(*model.Event)
		if !ok {
			return true
		}

		s := serializers.NewEventSerializer(ev, p.GetResolvers())
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
