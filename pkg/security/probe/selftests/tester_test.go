// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package selftests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// asyncSelfTest delivers its event after a delay, like the probe does once the triggering syscall has returned
type asyncSelfTest struct {
	tester    *SelfTester
	ruleID    eval.RuleID
	delay     time.Duration
	isSuccess bool
}

func (a *asyncSelfTest) GetRuleDefinition() *rules.RuleDefinition {
	return &rules.RuleDefinition{ID: a.ruleID}
}

func (a *asyncSelfTest) GenerateEvent(_ context.Context) error {
	a.isSuccess = false
	go func() {
		time.Sleep(a.delay)
		// same gating as IsExpectedEvent
		if a.tester.waitingForEvent.Load() {
			select {
			case a.tester.eventChan <- selfTestEvent{RuleID: a.ruleID}:
			default:
			}
		}
	}()
	return nil
}

func (a *asyncSelfTest) HandleEvent(event selfTestEvent) {
	a.isSuccess = event.RuleID == a.ruleID
}

func (a *asyncSelfTest) IsSuccess() bool {
	return a.isSuccess
}

func TestRunStartedFromResultCallback(t *testing.T) {
	tester := &SelfTester{
		waitingForEvent: atomic.NewBool(false),
		eventChan:       make(chan selfTestEvent, 1),
		selfTestRunning: make(chan time.Duration, 10),
		done:            make(chan bool),
		errorTimestamp:  make(map[eval.RuleID]time.Time),
	}
	tester.selfTests = []SelfTest{&asyncSelfTest{tester: tester, ruleID: "async", delay: 50 * time.Millisecond}}
	t.Cleanup(func() { _ = tester.Close() })

	const timeout = 5 * time.Second
	results := make(chan []eval.RuleID, 2)
	runs := 0
	go tester.WaitForResult(func(_ []eval.RuleID, fails []eval.RuleID) {
		results <- fails
		runs++
		// the CWS consumer starts the next run from this callback
		if runs == 1 {
			assert.NoError(t, tester.RunSelfTest(context.Background(), timeout))
		}
	})

	require.NoError(t, tester.RunSelfTest(context.Background(), timeout))

	for run := 1; run <= 2; run++ {
		select {
		case fails := <-results:
			assert.Empty(t, fails, "run %d", run)
		case <-time.After(2 * timeout):
			t.Fatalf("run %d didn't report", run)
		}
	}
}
