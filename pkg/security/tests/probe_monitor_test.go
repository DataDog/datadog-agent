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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestRulesetLoaded(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "path_test",
		Expression: `open.file.path =~ "*a/test-open" && open.flags & O_CREAT != 0`,
	}

	probeMonitorOpts := testOpts{}
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, probeMonitorOpts)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("ruleset_loaded", func(t *testing.T) {
		if err := test.GetProbeCustomEvent(t, func() error {
			test.reloadConfiguration()
			return nil
		}, func(rule *rules.Rule, customEvent *sprobe.CustomEvent) bool {
			assert.Equal(t, probe.RulesetLoadedRuleID, rule.ID, "wrong rule")
			return true
		}, model.CustomRulesetLoadedEventType); err != nil {
			t.Error(err)
		}
	})
}

func truncatedParents(t *testing.T, opts testOpts) {
	var truncatedParents string
	for i := 0; i < model.MaxPathDepth; i++ {
		truncatedParents += "a/"
	}

	rule := &rules.RuleDefinition{
		ID:         "path_test",
		Expression: `open.file.path =~ "*/a" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, opts)
	if err != nil {
		t.Fatal(err)
	}

	truncatedParentsFile, _, err := test.Path(fmt.Sprintf("%s", truncatedParents))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("truncated_parents", func(t *testing.T) {
		if os.MkdirAll(path.Dir(truncatedParentsFile), 0755) != nil {
			t.Fatal(err)
		}

		defer os.Remove(truncatedParentsFile)

		err = test.GetProbeCustomEvent(t, func() error {
			f, err := os.OpenFile(truncatedParentsFile, os.O_CREATE, 0755)
			if err != nil {
				t.Fatal(err)
			}
			return f.Close()
		}, func(rule *rules.Rule, customEvent *sprobe.CustomEvent) bool {
			assert.Equal(t, probe.AbnormalPathRuleID, rule.ID, "wrong rule")
			return true
		}, model.CustomTruncatedParentsEventType)
		if err != nil {
			t.Error(err)
		}

		err = test.GetSignal(t, func() error {
			f, err := os.OpenFile(truncatedParentsFile, os.O_CREATE, 0755)
			if err != nil {
				t.Fatal(err)
			}
			return f.Close()
		}, func(event *sprobe.Event, rule *rules.Rule) {
			// check the length of the filepath that triggered the custom event
			filepath, err := event.GetFieldValue("open.file.path")
			if err == nil {
				splittedFilepath := strings.Split(filepath.(string), "/")
				for len(splittedFilepath) > 1 && splittedFilepath[0] != "a" {
					// Remove the initial "" and all subsequent parents introduced by the mount point, we only want to
					// count the "a"s.
					splittedFilepath = splittedFilepath[1:]
				}
				assert.Equal(t, "a", splittedFilepath[0], "invalid path resolution at the left edge")
				assert.Equal(t, "a", splittedFilepath[len(splittedFilepath)-1], "invalid path resolution at the right edge")
				assert.Equal(t, model.MaxPathDepth, len(splittedFilepath), "invalid path depth")
			}
		})
		if err != nil {
			t.Error(err)
		}
	})
}

func TestTruncatedParentsMap(t *testing.T) {
	truncatedParents(t, testOpts{disableERPCDentryResolution: true})
}

func TestTruncatedParentsERPC(t *testing.T) {
	truncatedParents(t, testOpts{disableMapDentryResolution: true})
}

func TestNoisyProcess(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "path_test",
		Expression: `open.file.path =~ "*do-not-match/test-open" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{disableDiscarders: true, eventsCountThreshold: 1000})
	if err != nil {
		t.Fatal(err)
	}

	file, _, err := test.Path("test-open")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("noisy_process", func(t *testing.T) {
		err = test.GetProbeCustomEvent(t, func() error {
			// generate load
			for i := int64(0); i < testMod.config.LoadControllerEventsCountThreshold*2; i++ {
				f, err := os.OpenFile(file, os.O_CREATE, 0755)
				if err != nil {
					t.Fatal(err)
				}
				_ = f.Close()
				_ = os.Remove(file)
			}
			return nil
		}, func(rule *rules.Rule, customEvent *sprobe.CustomEvent) bool {
			assert.Equal(t, probe.NoisyProcessRuleID, rule.ID, "wrong rule")
			return true
		}, model.CustomNoisyProcessEventType)
		if err != nil {
			t.Error(err)
		}

		// make sure the discarder has expired before moving on to other tests
		t.Logf("waiting for the discarder to expire (%s)", testMod.config.LoadControllerDiscarderTimeout)
		time.Sleep(testMod.config.LoadControllerDiscarderTimeout + 1*time.Second)
	})
}
