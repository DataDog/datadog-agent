// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

// Package selftests holds selftests related files
package selftests

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
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

	"golang.org/x/sys/windows/registry"
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

	waitingForEvent *atomic.Bool
	eventChan       chan selfTestEvent
	probe           *probe.Probe
	success         []eval.RuleID
	fails           []eval.RuleID
	lastTimestamp   time.Time
	selfTests       []SelfTest
	tmpDir          string
	tmpKey          string
	done            chan bool
}

var _ rules.PolicyProvider = (*SelfTester)(nil)

// NewSelfTester returns a new SelfTester, enabled or not
func NewSelfTester(cfg *config.RuntimeSecurityConfig, probe *probe.Probe) (*SelfTester, error) {
	var (
		selfTests []SelfTest
		tmpDir    string
		tmpKey    string
	)

	if runtime.GOOS == "windows" {
		dir, err := createTargetDir()
		if err != nil {
			return nil, err
		}
		tmpDir = dir
		fileToCreate := "file.txt"

		keyName, err := createTempRegistryKey()
		if err != nil {
			return nil, err
		}
		tmpKey = keyName
		selfTests = []SelfTest{
			&WindowsCreateFileSelfTest{filename: fmt.Sprintf("%s/%s", dir, fileToCreate)},
			&WindowsSetRegistryKeyTest{keyName: keyName},
		}
	}

	s := &SelfTester{
		waitingForEvent: atomic.NewBool(cfg.EBPFLessEnabled),
		eventChan:       make(chan selfTestEvent, 10),
		probe:           probe,
		selfTests:       selfTests,
		tmpDir:          tmpDir,
		tmpKey:          tmpKey,
		done:            make(chan bool),
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

func createTargetDir() (string, error) {
	// Create temp directory to put target file in
	tmpDir, err := os.MkdirTemp("", "datadog_agent_cws_self_test")
	if err != nil {
		return "", err
	}
	return tmpDir, nil
}

func createTempRegistryKey() (string, error) {

	keyName := fmt.Sprintf("datadog_agent_cws_self_test_temp_registry_key_%s", strconv.FormatInt(time.Now().UnixNano(), 10))

	baseKey, err := registry.OpenKey(registry.CURRENT_USER, "Software", registry.WRITE)
	if err != nil {
		return "", fmt.Errorf("failed to open base key: %v", err)
	}
	defer baseKey.Close()

	// Create the temporary subkey under HKEY_CURRENT_USER\Software
	tempKey, _, err := registry.CreateKey(baseKey, keyName, registry.ALL_ACCESS)
	if err != nil {
		return "", fmt.Errorf("failed to create temporary registry key: %v", err)
	}
	tempKey.Close()

	// Return the full path of the created temporary registry key
	return fmt.Sprintf("HKCU:\\Software\\%s", keyName), nil
}

func deleteRegistryKey(path string) error {
	// Open base key
	baseKey, err := registry.OpenKey(registry.CURRENT_USER, "Software", registry.WRITE)
	if err != nil {
		return fmt.Errorf("failed to open base key: %v", err)
	}
	defer baseKey.Close()

	// Delete the registry key
	if err := registry.DeleteKey(baseKey, path); err != nil {
		return fmt.Errorf("failed to delete registry key: %v", err)
	}

	return nil
}

// RunSelfTest runs the self test and return the result
func (t *SelfTester) RunSelfTest(_ time.Duration) error {
	t.Lock()
	defer t.Unlock()

	t.beginSelfTests()

	for _, selfTest := range t.selfTests {
		if err := selfTest.GenerateEvent(); err != nil {
			log.Errorf("self test failed: %s", selfTest.GetRuleDefinition().ID)
		}
	}

	return nil
}

// Start starts the self tester policy provider
func (t *SelfTester) Start() {}

// WaitForResult wait for self test results
func (t *SelfTester) WaitForResult(timeout time.Duration, cb func(success []eval.RuleID, fails []eval.RuleID, events map[eval.RuleID]*serializers.EventSerializer)) {
	timer := time.After(timeout)

	var (
		success []string
		fails   []string
		events  = make(map[eval.RuleID]*serializers.EventSerializer)
	)

	for {
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
	close(t.done)
	if t.tmpDir != "" {
		err := os.RemoveAll(t.tmpDir)
		t.tmpDir = ""
		return err
	}
	if t.tmpKey != "" {
		err := deleteRegistryKey(t.tmpKey)
		t.tmpKey = ""
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

func (t *SelfTester) beginSelfTests() {
	t.waitingForEvent.Store(true)
}

func (t *SelfTester) endSelfTests() {
	t.waitingForEvent.Store(false)
}

type selfTestEvent struct {
	RuleID eval.RuleID
	Event  *serializers.EventSerializer
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
			RuleID: rule.ID,
			Event:  s,
		}

		t.eventChan <- selfTestEvent
		return true
	}
	return false
}

// WaitForResult wait for self test results
func (t *SelfTester) WaitForResult(_ func(_ []eval.RuleID, _ []eval.RuleID, _ map[eval.RuleID]*serializers.EventSerializer)) {
}
