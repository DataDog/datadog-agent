// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

package tests

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestRulesetLoaded(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "path_test",
		Expression: `open.file.path == "/aaaaaaaaaaaaaaaaaaaaaaaaa" && open.flags & O_CREAT != 0`,
	}

	probeMonitorOpts := testOpts{}
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, probeMonitorOpts)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	test.cws.SendStats()

	key := metrics.MetricRuleSetLoaded
	assert.NotEmpty(t, test.statsdClient.Get(key))
	assert.NotZero(t, test.statsdClient.Get(key))

	test.statsdClient.Flush()

	t.Run("ruleset_loaded", func(t *testing.T) {
		count := test.statsdClient.Get(key)
		assert.Zero(t, count)

		err = test.GetCustomEventSent(t, func() error {
			// force a reload
			return syscall.Kill(syscall.Getpid(), syscall.SIGHUP)
		}, func(rule *rules.Rule, customEvent *events.CustomEvent) bool {
			assert.Equal(t, events.RulesetLoadedRuleID, rule.ID, "wrong rule")

			test.cws.SendStats()

			assert.Equal(t, count+1, test.statsdClient.Get(key))

			return validateRuleSetLoadedSchema(t, customEvent)
		}, 20*time.Second, model.CustomRulesetLoadedEventType)
		if err != nil {
			t.Fatal(err)
		}
	})
}

func truncatedParents(t *testing.T, opts testOpts) {
	var truncatedParents string
	for i := 0; i < model.MaxPathDepth; i++ {
		truncatedParents += "a/"
	}

	rule := &rules.RuleDefinition{
		ID: "path_test",
		// because of the truncated path open.file.path will be '/a/a/a/a*' and not '{{.Root}}/a/a/a*'
		Expression: `open.file.path =~ "*/a/**" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	truncatedParentsFile, _, err := test.Path(truncatedParents)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("truncated_parents", func(t *testing.T) {
		if os.MkdirAll(path.Dir(truncatedParentsFile), 0755) != nil {
			t.Fatal(err)
		}

		// By default, the `t.TempDir` cleanup has a bit of a hard time cleaning up such a deep file
		// let's help it by cleaning up most of the directories
		defer cleanupABottomUp(truncatedParentsFile)

		err = test.GetCustomEventSent(t, func() error {
			f, err := os.OpenFile(truncatedParentsFile, os.O_CREATE, 0755)
			if err != nil {
				return err
			}
			return f.Close()
		}, func(rule *rules.Rule, customEvent *events.CustomEvent) bool {
			assert.Equal(t, events.AbnormalPathRuleID, rule.ID, "wrong rule")
			return true
		}, getEventTimeout, model.CustomTruncatedParentsEventType)
		if err != nil {
			t.Fatal(err)
		}

		test.WaitSignal(t, func() error {
			f, err := os.OpenFile(truncatedParentsFile, os.O_CREATE, 0755)
			if err != nil {
				return err
			}
			return f.Close()
		}, func(event *model.Event, rule *rules.Rule) {
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
	})
}

func cleanupABottomUp(path string) {
	for filepath.Base(path) == "a" {
		os.RemoveAll(path)
		path = filepath.Dir(path)
	}
}

func TestTruncatedParentsMap(t *testing.T) {
	truncatedParents(t, testOpts{disableERPCDentryResolution: true, disableAbnormalPathCheck: true})
}

func TestTruncatedParentsERPC(t *testing.T) {
	truncatedParents(t, testOpts{disableMapDentryResolution: true, disableAbnormalPathCheck: true})
}

func TestNoisyProcess(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID: "path_test",
		// use the basename as an approver. The rule won't match as the parent folder differs but we will get the event because of the approver.
		Expression: `open.file.path == "{{.Root}}/do_not_match/test-open"`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{disableDiscarders: true, eventsCountThreshold: 1000})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	file, _, err := test.Path("test-open")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("noisy_process", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			// generate load
			for i := int64(0); i < testMod.secconfig.Probe.LoadControllerEventsCountThreshold*2; i++ {
				f, err := os.OpenFile(file, os.O_CREATE, 0755)
				if err != nil {
					return err
				}
				if err = f.Close(); err != nil {
					return err
				}
				if err = os.Remove(file); err != nil {
					return err
				}
			}
			return nil
		}, func(rule *rules.Rule, customEvent *events.CustomEvent) bool {
			assert.Equal(t, events.NoisyProcessRuleID, rule.ID, "wrong rule")
			return true
		}, getEventTimeout, model.CustomNoisyProcessEventType)
		if err != nil {
			t.Fatal(err)
		}

		// make sure the discarder has expired before moving on to other tests
		t.Logf("waiting for the discarder to expire (%s)", testMod.secconfig.Probe.LoadControllerDiscarderTimeout)
		time.Sleep(testMod.secconfig.Probe.LoadControllerDiscarderTimeout + 1*time.Second)
	})
}
