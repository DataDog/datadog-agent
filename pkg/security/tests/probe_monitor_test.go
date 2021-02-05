// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"fmt"
	"os"
	"path"
	"syscall"
	"testing"
	"time"

	"github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestProbeMonitor(t *testing.T) {
	var truncatedParents, truncatedSegment string
	for i := 0; i <= probe.MaxPathDepth; i++ {
		truncatedParents += "a/"
	}
	for i := 0; i <= probe.MaxSegmentLength+1; i++ {
		truncatedSegment += "a"
	}

	rule := &rules.RuleDefinition{
		ID:         "path_test",
		Expression: `open.filename =~ "*a/test-open" && open.flags & O_CREAT != 0`,
	}

	probeMonitorOpts := testOpts{}
	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, probeMonitorOpts)
	if err != nil {
		t.Fatal(err)
	}

	truncatedParentsFile, truncatedParentsFilePtr, err := test.Path(fmt.Sprintf("%stest-open", truncatedParents))
	if err != nil {
		t.Fatal(err)
	}

	truncatedSegmentFile, truncatedSegmentFilePtr, err := test.Path(fmt.Sprintf("%s/test-open", truncatedSegment))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("ruleset_loaded", func(t *testing.T) {
		test.Close()
		test, err = newTestModule(nil, []*rules.RuleDefinition{rule}, probeMonitorOpts)
		if err != nil {
			t.Fatal(err)
		}
		defer test.Close()

		ruleEvent, err := test.GetProbeCustomEvent(1*time.Second, probe.CustomRulesetLoadedEventType.String())
		if err != nil {
			t.Error(err)
		} else {
			if ruleEvent.RuleID != probe.RulesetLoadedRuleID {
				t.Errorf("expected %s rule, got %s", probe.RulesetLoadedRuleID, ruleEvent.RuleID)
			}
		}
	})

	t.Run("truncated_segment", func(t *testing.T) {
		if os.MkdirAll(path.Dir(truncatedSegmentFile), 0755) != nil {
			t.Fatal(err)
		}
		fd, _, errno := syscall.Syscall(syscall.SYS_OPEN, uintptr(truncatedSegmentFilePtr), syscall.O_CREAT, 0755)
		if errno != 0 {
			t.Fatal(error(errno))
		}
		defer os.Remove(truncatedSegmentFile)
		defer syscall.Close(int(fd))

		ruleEvent, err := test.GetProbeCustomEvent(3*time.Second, probe.CustomTruncatedSegmentEventType.String())
		if err != nil {
			t.Error(err)
		} else {
			if ruleEvent.RuleID != probe.AbnormalPathRuleID {
				t.Errorf("expected %s rule, got %s", probe.AbnormalPathRuleID, ruleEvent.RuleID)
			}
		}
	})

	t.Run("truncated_parents", func(t *testing.T) {
		if os.MkdirAll(path.Dir(truncatedParentsFile), 0755) != nil {
			t.Fatal(err)
		}
		fd, _, errno := syscall.Syscall(syscall.SYS_OPEN, uintptr(truncatedParentsFilePtr), syscall.O_CREAT, 0755)
		if errno != 0 {
			t.Fatal(error(errno))
		}
		defer os.Remove(truncatedParentsFile)
		defer syscall.Close(int(fd))

		ruleEvent, err := test.GetProbeCustomEvent(3*time.Second, probe.CustomTruncatedParentsEventType.String())
		if err != nil {
			t.Error(err)
		} else {
			if ruleEvent.RuleID != probe.AbnormalPathRuleID {
				t.Errorf("expected %s rule, got %s", probe.AbnormalPathRuleID, ruleEvent.RuleID)
			}
		}
	})
}

func TestNoisyProcess(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "path_test",
		Expression: `open.filename =~ "*do-not-match/test-open" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{disableDiscarders: true, eventsCountThreshold: 1000})
	if err != nil {
		t.Fatal(err)
	}

	file, filePtr, err := test.Path("test-open")
	if err != nil {
		t.Fatal(err)
	}

	prevLevel, err := test.st.swapLogLevel(seelog.WarnLvl)
	if err != nil {
		t.Error(err)
	}

	t.Run("noisy_process", func(t *testing.T) {
		// generate load
		for i := int64(0); i < testMod.config.LoadControllerEventsCountThreshold*2; i++ {
			fd, _, errno := syscall.Syscall(syscall.SYS_OPEN, uintptr(filePtr), syscall.O_CREAT, 0755)
			if errno != 0 {
				t.Fatal(error(errno))
			}
			_ = syscall.Close(int(fd))
			_ = os.Remove(file)
		}

		ruleEvent, err := test.GetProbeCustomEvent(1*time.Second, probe.CustomNoisyProcessEventType.String())
		if err != nil {
			t.Error(err)
		} else {
			if ruleEvent.RuleID != probe.NoisyProcessRuleID {
				t.Errorf("expected %s rule, got %s", probe.NoisyProcessRuleID, ruleEvent.RuleID)
			}
		}

		// make sure the discarder has expired before moving on to other tests
		t.Logf("waiting for the discarder to expire (%s)", testMod.config.LoadControllerDiscarderTimeout)
		time.Sleep(testMod.config.LoadControllerDiscarderTimeout + 1*time.Second)
	})

	if _, err := test.st.swapLogLevel(prevLevel); err != nil {
		t.Error(err)
	}
}
