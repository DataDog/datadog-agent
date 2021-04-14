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
	"testing"
	"time"

	"github.com/cihub/seelog"
	"gotest.tools/assert"

	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestProbeMonitor(t *testing.T) {
	var truncatedParents, truncatedSegment string
	for i := 0; i <= model.MaxPathDepth; i++ {
		truncatedParents += "a/"
	}
	for i := 0; i <= model.MaxSegmentLength+1; i++ {
		truncatedSegment += "a"
	}

	rule := &rules.RuleDefinition{
		ID:         "path_test",
		Expression: `open.file.path =~ "*a/test-open" && open.flags & O_CREAT != 0`,
	}

	probeMonitorOpts := testOpts{}
	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, probeMonitorOpts)
	if err != nil {
		t.Fatal(err)
	}

	truncatedParentsFile, _, err := test.Path(fmt.Sprintf("%stest-open", truncatedParents))
	if err != nil {
		t.Fatal(err)
	}

	truncatedSegmentFile, _, err := test.Path(fmt.Sprintf("%s/test-open", truncatedSegment))
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

		ruleEvent, err := test.GetProbeCustomEvent(1*time.Second, model.CustomRulesetLoadedEventType.String())
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, ruleEvent.RuleID, probe.RulesetLoadedRuleID, "wrong rule")
		}
	})

	t.Run("truncated_segment", func(t *testing.T) {
		if os.MkdirAll(path.Dir(truncatedSegmentFile), 0755) != nil {
			t.Fatal(err)
		}

		f, err := os.OpenFile(truncatedSegmentFile, os.O_CREATE, 0755)
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(truncatedSegmentFile)
		defer f.Close()

		ruleEvent, err := test.GetProbeCustomEvent(3*time.Second, model.CustomTruncatedSegmentEventType.String())
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, ruleEvent.RuleID, probe.AbnormalPathRuleID, "wrong rule")
		}
	})

	t.Run("truncated_parents", func(t *testing.T) {
		if os.MkdirAll(path.Dir(truncatedParentsFile), 0755) != nil {
			t.Fatal(err)
		}

		f, err := os.OpenFile(truncatedParentsFile, os.O_CREATE, 0755)
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(truncatedParentsFile)
		defer f.Close()

		ruleEvent, err := test.GetProbeCustomEvent(3*time.Second, model.CustomTruncatedParentsEventType.String())
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, ruleEvent.RuleID, probe.AbnormalPathRuleID, "wrong rule")
		}
	})
}

func TestNoisyProcess(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "path_test",
		Expression: `open.file.path =~ "*do-not-match/test-open" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{disableDiscarders: true, eventsCountThreshold: 1000})
	if err != nil {
		t.Fatal(err)
	}

	file, _, err := test.Path("test-open")
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
			f, err := os.OpenFile(file, os.O_CREATE, 0755)
			if err != nil {
				t.Fatal(err)
			}
			_ = f.Close()
			_ = os.Remove(file)
		}

		ruleEvent, err := test.GetProbeCustomEvent(1*time.Second, model.CustomNoisyProcessEventType.String())
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, ruleEvent.RuleID, probe.NoisyProcessRuleID, "wrong rule")
		}

		// make sure the discarder has expired before moving on to other tests
		t.Logf("waiting for the discarder to expire (%s)", testMod.config.LoadControllerDiscarderTimeout)
		time.Sleep(testMod.config.LoadControllerDiscarderTimeout + 1*time.Second)
	})

	if _, err := test.st.swapLogLevel(prevLevel); err != nil {
		t.Error(err)
	}
}
