// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"context"
	"fmt"
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
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

func TestEventRulesetLoaded(t *testing.T) {
	SkipIfNotAvailable(t)

	rule := &rules.RuleDefinition{
		ID:         "path_test",
		Expression: `open.file.path == "/aaaaaaaaaaaaaaaaaaaaaaaaa" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
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

func TestEventHeartbeatSent(t *testing.T) {
	SkipIfNotAvailable(t)

	rule := &rules.RuleDefinition{
		ID:         "path_test",
		Expression: `open.file.path == "/aaaaaaaaaaaaaaaaaaaaaaaaa" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	test.cws.SendStats()

	t.Run("heartbeat", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			// force a reload
			return syscall.Kill(syscall.Getpid(), syscall.SIGHUP)
		}, func(rule *rules.Rule, customEvent *events.CustomEvent) bool {

			isHeartbeatEvent := events.HeartbeatRuleID == rule.ID

			return validateHeartbeatSchema(t, customEvent) && isHeartbeatEvent
		}, 80*time.Second, model.CustomHeartbeatEventType)
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestEventRaleLimiters(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:               "test_unique_id",
			Expression:       `open.file.path == "{{.Root}}/test-unique-id"`,
			Every:            5 * time.Second,
			RateLimiterToken: []string{"process.file.name"},
		},
		{
			ID:         "test_std",
			Expression: `open.file.path == "{{.Root}}/test-std"`,
			Every:      5 * time.Second,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("token", func(t *testing.T) {
		testFile, _, err := test.Path("test-unique-id")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		err = test.GetEventSent(t, func() error {
			f, err := os.OpenFile(testFile, os.O_CREATE, 0)
			if err != nil {
				t.Fatal(err)
			}
			return f.Close()
		}, func(rule *rules.Rule, event *model.Event) bool {
			return true
		}, time.Second*3, "test_unique_id")
		if err != nil {
			t.Error(err)
		}

		// open from another process
		err = test.GetEventSent(t, func() error {
			timeoutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			return runSyscallTesterFunc(
				timeoutCtx, t, syscallTester,
				"open", testFile,
			)
		}, func(rule *rules.Rule, event *model.Event) bool {
			return true
		}, time.Second*3, "test_unique_id")
		if err != nil {
			t.Error(err)
		}

		// open from the first process
		err = test.GetEventSent(t, func() error {
			f, err := os.OpenFile(testFile, os.O_CREATE, 0)
			if err != nil {
				t.Fatal(err)
			}
			return f.Close()
		}, func(rule *rules.Rule, event *model.Event) bool {
			return true
		}, time.Second*3, "test_unique_id")
		if err == nil {
			t.Error("unexpected event")
		}
	})

	t.Run("std", func(t *testing.T) {
		testFile, _, err := test.Path("test-std")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		err = test.GetEventSent(t, func() error {
			f, err := os.OpenFile(testFile, os.O_CREATE, 0)
			if err != nil {
				t.Fatal(err)
			}
			return f.Close()
		}, func(rule *rules.Rule, event *model.Event) bool {
			return true
		}, time.Second*3, "test_std")
		if err != nil {
			t.Error(err)
		}

		// open from another process
		err = test.GetEventSent(t, func() error {
			timeoutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			return runSyscallTesterFunc(
				timeoutCtx, t, syscallTester,
				"open", testFile,
			)
		}, func(rule *rules.Rule, event *model.Event) bool {
			return true
		}, time.Second*3, "test_std")
		if err == nil {
			t.Error(err)
		}
	})
}

func TestEventIteratorRegister(t *testing.T) {
	SkipIfNotAvailable(t)

	pid1ExePath := utils.ProcExePath(1)
	pid1Path, err := os.Readlink(pid1ExePath)
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_register_1",
			Expression: `open.file.path == "{{.Root}}/test-register" && process.ancestors[A].name == "syscall_tester" && process.ancestors[A].argv in ["span-exec"]`,
		},
		{
			ID:         "test_register_2",
			Expression: fmt.Sprintf(`open.file.path == "{{.Root}}/test-register-2" && process.ancestors[A].file.path == "%s" && process.ancestors[A].pid == 1`, pid1Path),
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("test-register")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	testFile2, _, err := test.Path("test-register-2")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile2)

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("std", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "span-exec", "123", "456", "/usr/bin/touch", testFile)
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_register_1")
		})
	})

	t.Run("pid1", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			f, err := os.Create(testFile2)
			if err != nil {
				return err
			}
			return f.Close()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_register_2")
		})
	})
}

func truncatedParents(t *testing.T, staticOpts testOpts, dynamicOpts dynamicTestOpts) {
	var truncatedParents string
	for i := 0; i < model.MaxPathDepth; i++ {
		truncatedParents += "a/"
	}

	rule := &rules.RuleDefinition{
		ID: "path_test",
		// because of the truncated path open.file.path will be '/a/a/a/a*' and not '{{.Root}}/a/a/a*'
		Expression: `open.file.path =~ "*/a/**" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, withStaticOpts(staticOpts), withDynamicOpts(dynamicOpts))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	truncatedParentsFile, _, err := test.Path(truncatedParents)
	if err != nil {
		t.Fatal(err)
	}

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
}

func cleanupABottomUp(path string) {
	for filepath.Base(path) == "a" {
		os.RemoveAll(path)
		path = filepath.Dir(path)
	}
}

func TestEventTruncatedParents(t *testing.T) {
	SkipIfNotAvailable(t)

	t.Run("map", func(t *testing.T) {
		truncatedParents(t, testOpts{disableERPCDentryResolution: true}, dynamicTestOpts{disableAbnormalPathCheck: true})
	})

	t.Run("erpc", func(t *testing.T) {
		truncatedParents(t, testOpts{disableMapDentryResolution: true}, dynamicTestOpts{disableAbnormalPathCheck: true})
	})
}
