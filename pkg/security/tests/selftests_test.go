// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files

package tests

import (
	"errors"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"

	"github.com/avast/retry-go/v4"
	"github.com/oliveagle/jsonpath"
	"github.com/stretchr/testify/assert"
)

func TestSelfTests(t *testing.T) {
	SkipIfNotAvailable(t)
	flake.MarkOnJobName(t, "ubuntu_25.10")

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(testOpts{enableSelfTests: true}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	test.msgSender.flush()

	err = retry.Do(func() error {
		msg := test.msgSender.getMsg(events.SelfTestRuleID)
		if msg == nil {
			return errors.New("self_test event not found")
		}

		log.Debug("self_tests event tags:", msg.Tags)
		assert.NotEmpty(t, msg.Tags, "event's tags are empty")

		jsonPathValidation(test, msg.Data, func(_ *testModule, obj interface{}) {
			succeededTests, err := jsonpath.JsonPathLookup(obj, `$.succeeded_tests`)
			if err != nil {
				t.Errorf("could not get succeeded_tests field: %v", err)
			}
			failedTests, err := jsonpath.JsonPathLookup(obj, `$.failed_tests`)
			if err != nil {
				t.Errorf("could not get failed_tests field: %v", err)
			}

			if len(succeededTests.([]interface{})) != 3 || len(failedTests.([]interface{})) > 0 {
				t.Errorf("test results: successes: %v, fails: %v", succeededTests, failedTests)
			}

		})

		return nil
	}, retry.Attempts(20), retry.Delay(2*time.Second), retry.DelayType(retry.FixedDelay))

	assert.NoError(t, err)
}
