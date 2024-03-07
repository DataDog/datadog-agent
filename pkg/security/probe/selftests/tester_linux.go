// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package selftests holds selftests related files
package selftests

import (
	"os"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SelfTest represent one self test
type SelfTest interface {
	GetRuleDefinition() *rules.RuleDefinition
	GenerateEvent() error
	HandleEvent(selfTestEvent)
	IsSuccess() bool
}

// SelfTester represents all the state needed to conduct rule injection test at startup
type SelfTester struct {
	sync.Mutex

	config          *config.RuntimeSecurityConfig
	waitingForEvent *atomic.Bool
	eventChan       chan selfTestEvent
	probe           *probe.Probe
	success         []eval.RuleID
	fails           []eval.RuleID
	lastTimestamp   time.Time
	selfTests       []SelfTest
	tmpDir          string
	done            chan bool
	selfTestRunning chan time.Duration
}

var _ rules.PolicyProvider = (*SelfTester)(nil)

// NewSelfTester returns a new SelfTester, enabled or not
func NewSelfTester(cfg *config.RuntimeSecurityConfig, probe *probe.Probe) (*SelfTester, error) {
	var (
		selfTests []SelfTest
		tmpDir    string
	)

	if cfg.EBPFLessEnabled {
		selfTests = []SelfTest{
			&EBPFLessSelfTest{},
		}
	} else {
		name, dir, err := createTargetFile()
		if err != nil {
			return nil, err
		}
		tmpDir = dir

		selfTests = []SelfTest{
			&OpenSelfTest{filename: name},
			&ChmodSelfTest{filename: name},
			&ChownSelfTest{filename: name},
		}
	}

	s := &SelfTester{
		waitingForEvent: atomic.NewBool(cfg.EBPFLessEnabled),
		eventChan:       make(chan selfTestEvent, 10),
		selfTestRunning: make(chan time.Duration, 10),
		probe:           probe,
		selfTests:       selfTests,
		tmpDir:          tmpDir,
		done:            make(chan bool),
		config:          cfg,
	}

	return s, nil
}

// GetStatus returns the result of the last performed self tests
func (t *SelfTester) GetStatus() *api.SelfTestsStatus {
	t.Lock()
	defer t.Unlock()

	return &api.SelfTestsStatus{
		LastTimestamp: t.lastTimestamp.Format(time.RFC822),
		Success:       t.success,
		Fails:         t.fails,
	}
}

func createTargetFile() (string, string, error) {
	// Create temp directory to put target file in
	tmpDir, err := os.MkdirTemp("", "datadog_agent_cws_self_test")
	if err != nil {
		return "", "", err
	}

	// Create target file
	targetFile, err := os.CreateTemp(tmpDir, "datadog_agent_cws_target_file")
	if err != nil {
		return "", "", err
	}

	return targetFile.Name(), tmpDir, targetFile.Close()
}

// RunSelfTest runs the self test and return the result
func (t *SelfTester) RunSelfTest(timeout time.Duration) error {
	t.Lock()
	defer t.Unlock()

	t.beginSelfTests(timeout)

	for _, selfTest := range t.selfTests {
		if err := selfTest.GenerateEvent(); err != nil {
			log.Errorf("self test failed: %s", selfTest.GetRuleDefinition().ID)
		}
	}

	return nil
}

// Start the self tester policy provider
func (t *SelfTester) Start() {
	if t.config.EBPFLessEnabled {
		t.selfTestRunning <- DefaultTimeout
	}
}

// WaitForResult wait for self test results
func (t *SelfTester) WaitForResult(cb func(success []eval.RuleID, fails []eval.RuleID, events map[eval.RuleID]*serializers.EventSerializer)) {
	for timeout := range t.selfTestRunning {
		timer := time.After(timeout)

		var (
			success []string
			fails   []string
			events  = make(map[eval.RuleID]*serializers.EventSerializer)
		)

	LOOP:
		for {
			select {
			case <-t.done:
				return
			case event := <-t.eventChan:
				t.Lock()
				for _, selfTest := range t.selfTests {
					if !selfTest.IsSuccess() {
						selfTest.HandleEvent(event)
					}

					if selfTest.IsSuccess() {
						id := selfTest.GetRuleDefinition().ID
						events[id] = event.Event
					}
				}
				t.Unlock()

				// all test passed
				if len(events) == len(t.selfTests) {
					break LOOP
				}
			case <-timer:
				break LOOP
			}
		}

		t.Lock()
		for _, selfTest := range t.selfTests {
			id := selfTest.GetRuleDefinition().ID

			if _, ok := events[id]; ok {
				success = append(success, id)
			} else {
				fails = append(fails, id)
			}
		}
		t.success, t.fails, t.lastTimestamp = success, fails, time.Now()
		t.Unlock()

		cb(success, fails, events)

		t.endSelfTests()
	}
}

// Close removes temp directories and files used by the self tester
func (t *SelfTester) Close() error {
	close(t.selfTestRunning)
	close(t.done)

	if t.tmpDir != "" {
		err := os.RemoveAll(t.tmpDir)
		t.tmpDir = ""
		return err
	}
	return nil
}

// LoadPolicies implements the PolicyProvider interface
func (t *SelfTester) LoadPolicies(_ []rules.MacroFilter, _ []rules.RuleFilter) ([]*rules.Policy, *multierror.Error) {
	t.Lock()
	defer t.Unlock()
	p := &rules.Policy{
		Name:       policyName,
		Source:     policySource,
		Version:    policyVersion,
		IsInternal: true,
	}

	for _, selftest := range t.selfTests {
		p.AddRule(selftest.GetRuleDefinition())
	}

	return []*rules.Policy{p}, nil
}

func (t *SelfTester) beginSelfTests(timeout time.Duration) {
	t.waitingForEvent.Store(true)
	t.selfTestRunning <- timeout
}

func (t *SelfTester) endSelfTests() {
	t.waitingForEvent.Store(false)
}

type selfTestEvent struct {
	RuleID   eval.RuleID
	Filepath string
	Event    *serializers.EventSerializer
}

// IsExpectedEvent sends an event to the tester
func (t *SelfTester) IsExpectedEvent(rule *rules.Rule, event eval.Event, _ *probe.Probe) bool {
	if t.waitingForEvent.Load() && rule.Definition.Policy.Source == policySource {
		ev, ok := event.(*model.Event)
		if !ok {
			return true
		}

		s := serializers.NewEventSerializer(ev, rule.Opts)
		if s == nil {
			return false
		}

		selfTestEvent := selfTestEvent{
			RuleID:   rule.ID,
			Filepath: s.FileEventSerializer.Path,
			Event:    s,
		}

		t.eventChan <- selfTestEvent
		return true
	}
	return false
}
