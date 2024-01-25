// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

// Package tests holds tests related files
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

func TestHeartbeatSent(t *testing.T) {
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
	SkipIfNotAvailable(t)

	truncatedParents(t, testOpts{disableERPCDentryResolution: true}, dynamicTestOpts{disableAbnormalPathCheck: true})
}

func TestTruncatedParentsERPC(t *testing.T) {
	SkipIfNotAvailable(t)

	truncatedParents(t, testOpts{disableMapDentryResolution: true}, dynamicTestOpts{disableAbnormalPathCheck: true})
}
