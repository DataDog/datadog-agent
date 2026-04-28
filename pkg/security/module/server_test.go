// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"slices"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// mockActionReport implements model.ActionReport for testing
type mockActionReport struct {
	maxRetry  int
	resolved  error
	matchRule bool
}

func (m *mockActionReport) IsResolved() error                 { return m.resolved }
func (m *mockActionReport) MaxRetry() int                     { return m.maxRetry }
func (m *mockActionReport) ToJSON() ([]byte, error)           { return nil, nil }
func (m *mockActionReport) IsMatchingRule(_ eval.RuleID) bool { return m.matchRule }

var _ model.ActionReport = (*mockActionReport)(nil)

func newTestAPIServer(threshold int) *APIServer {
	return &APIServer{
		cfg: &config.RuntimeSecurityConfig{
			EventRetryQueueThreshold: threshold,
		},
	}
}

func TestSlicesDeleteUntilFalse(t *testing.T) {
	t.Run("all true removes all", func(t *testing.T) {
		msgs := []*pendingMsg{{ruleID: "a"}, {ruleID: "b"}, {ruleID: "c"}}
		result := slicesDeleteUntilFalse(msgs, func(_ *pendingMsg) bool { return true })
		if len(result) != 0 {
			t.Fatalf("expected empty slice, got %d elements", len(result))
		}
	})

	t.Run("stops at first false", func(t *testing.T) {
		msgs := []*pendingMsg{{ruleID: "a"}, {ruleID: "b"}, {ruleID: "c"}}
		result := slicesDeleteUntilFalse(msgs, func(msg *pendingMsg) bool {
			return msg.ruleID == "a"
		})
		if len(result) != 2 || result[0].ruleID != "b" || result[1].ruleID != "c" {
			t.Fatalf("expected [b, c], got %v", result)
		}
	})

	t.Run("first false keeps all", func(t *testing.T) {
		msgs := []*pendingMsg{{ruleID: "a"}, {ruleID: "b"}}
		result := slicesDeleteUntilFalse(msgs, func(_ *pendingMsg) bool { return false })
		if len(result) != 2 {
			t.Fatalf("expected 2 elements, got %d", len(result))
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		result := slicesDeleteUntilFalse(nil, func(_ *pendingMsg) bool { return true })
		if result != nil {
			t.Fatalf("expected nil, got %v", result)
		}
	})
}

func TestPendingMsgGetMaxRetry(t *testing.T) {
	t.Run("no action reports returns default", func(t *testing.T) {
		msg := &pendingMsg{}
		if got := msg.getMaxRetry(); got != defaultMaxRetry {
			t.Fatalf("expected %d, got %d", defaultMaxRetry, got)
		}
	})

	t.Run("action report above default is used", func(t *testing.T) {
		msg := &pendingMsg{
			actionReports: []model.ActionReport{
				&mockActionReport{maxRetry: defaultMaxRetry + 5},
			},
		}
		if got := msg.getMaxRetry(); got != defaultMaxRetry+5 {
			t.Fatalf("expected %d, got %d", defaultMaxRetry+5, got)
		}
	})

	t.Run("action report below default uses default", func(t *testing.T) {
		msg := &pendingMsg{
			actionReports: []model.ActionReport{
				&mockActionReport{maxRetry: 1},
			},
		}
		if got := msg.getMaxRetry(); got != defaultMaxRetry {
			t.Fatalf("expected %d, got %d", defaultMaxRetry, got)
		}
	})

	t.Run("takes max across multiple reports", func(t *testing.T) {
		msg := &pendingMsg{
			actionReports: []model.ActionReport{
				&mockActionReport{maxRetry: defaultMaxRetry + 3},
				&mockActionReport{maxRetry: defaultMaxRetry + 10},
				&mockActionReport{maxRetry: defaultMaxRetry + 1},
			},
		}
		if got := msg.getMaxRetry(); got != defaultMaxRetry+10 {
			t.Fatalf("expected %d, got %d", defaultMaxRetry+10, got)
		}
	})
}

func TestDequeueRetryIncrement(t *testing.T) {
	server := newTestAPIServer(100)
	now := time.Now()
	msg := &pendingMsg{
		ruleID:    "test-rule",
		sendAfter: now.Add(-time.Second),
	}
	server.queue = []*pendingMsg{msg}

	callCount := 0
	server.dequeue(now, func(_ *pendingMsg, _ bool) bool {
		callCount++
		return false // signal retry needed
	})

	if callCount != 1 {
		t.Fatalf("expected callback called once, got %d", callCount)
	}
	if msg.retry != 1 {
		t.Fatalf("expected retry=1, got %d", msg.retry)
	}
	if len(server.queue) != 1 {
		t.Fatalf("expected message to remain in queue, got queue size %d", len(server.queue))
	}
	if !msg.sendAfter.After(now) {
		t.Fatal("expected sendAfter to be updated to a future time")
	}
}

func TestDequeueMaxRetryForcesRemoval(t *testing.T) {
	server := newTestAPIServer(100)
	now := time.Now()
	msg := &pendingMsg{
		ruleID:    "test-rule",
		retry:     defaultMaxRetry, // already at max
		sendAfter: now.Add(-time.Second),
	}
	server.queue = []*pendingMsg{msg}

	callCount := 0
	server.dequeue(now, func(_ *pendingMsg, _ bool) bool {
		callCount++
		return false // cb says retry, but max is reached
	})

	if callCount != 1 {
		t.Fatalf("expected callback called once, got %d", callCount)
	}
	// message must be removed even though cb returned false
	if len(server.queue) != 0 {
		t.Fatalf("expected empty queue after max retry reached, got %d", len(server.queue))
	}
}

func TestDequeueRespectsSendAfterWhenQueueSmall(t *testing.T) {
	// sendAfter in future + queue below threshold → message is held, cb not called
	server := newTestAPIServer(100)
	now := time.Now()
	msg := &pendingMsg{
		ruleID:    "test-rule",
		sendAfter: now.Add(time.Second),
	}
	server.queue = []*pendingMsg{msg}

	callCount := 0
	server.dequeue(now, func(_ *pendingMsg, _ bool) bool {
		callCount++
		return true
	})

	if callCount != 0 {
		t.Fatalf("expected callback not called while message is delayed, got %d", callCount)
	}
	if len(server.queue) != 1 {
		t.Fatalf("expected message to remain in queue, got %d", len(server.queue))
	}
}

func TestDequeueBypassesDelayWhenQueueFull(t *testing.T) {
	// queue size == threshold → delay is bypassed and isRetryAllowed=false
	server := newTestAPIServer(1) // threshold=1, queue will have 1 item → full
	now := time.Now()
	msg := &pendingMsg{
		ruleID:    "test-rule",
		sendAfter: now.Add(time.Second), // would normally be delayed
	}
	server.queue = []*pendingMsg{msg}

	var gotIsRetryAllowed bool
	callCount := 0
	server.dequeue(now, func(_ *pendingMsg, isRetryAllowed bool) bool {
		callCount++
		gotIsRetryAllowed = isRetryAllowed
		return true
	})

	if callCount != 1 {
		t.Fatalf("expected callback called once (delay bypassed), got %d", callCount)
	}
	if gotIsRetryAllowed {
		t.Fatal("expected isRetryAllowed=false when queue is at threshold")
	}
	if len(server.queue) != 0 {
		t.Fatalf("expected empty queue after message sent, got %d", len(server.queue))
	}
}

func TestExtTagsCbCalledWhenQueueFull(t *testing.T) {
	// The key fix: extTagsCb must be called even when the queue is full (isRetryAllowed=false).
	// Previously the callback was guarded by isRetryAllowed, so tags were never collected.
	server := newTestAPIServer(1) // threshold=1 → queue full with one item

	now := time.Now()
	cbCalled := false
	msg := &pendingMsg{
		ruleID:    "test-rule",
		sendAfter: now.Add(-time.Second),
		extTagsCb: func() ([]string, bool) {
			cbCalled = true
			return []string{"container:my-app"}, false
		},
	}
	server.queue = []*pendingMsg{msg}

	server.dequeue(now, server.tryResolve)

	if !cbCalled {
		t.Fatal("expected extTagsCb to be called even when queue is full")
	}
	if !slices.Contains(msg.tags, "container:my-app") {
		t.Fatalf("expected collected tag in message, got %v", msg.tags)
	}
	if len(server.queue) != 0 {
		t.Fatalf("expected message to be sent (removed from queue), got size %d", len(server.queue))
	}
	// tags were present, so no missing-tags or skipped-retry counter should fire
	if v := server.missingTagsCount.Load(); v != 0 {
		t.Fatalf("expected missingTagsCount=0, got %d", v)
	}
	if v := server.skippedRetryCount.Load(); v != 0 {
		t.Fatalf("expected skippedRetryCount=0, got %d", v)
	}
}

func TestExtTagsCbRetriesWhenEmptyAndRetryAllowed(t *testing.T) {
	// When extTagsCb returns empty tags + retryable=true + isRetryAllowed=true → retry
	server := newTestAPIServer(100) // high threshold → retry allowed

	now := time.Now()
	cbCallCount := 0
	msg := &pendingMsg{
		ruleID:    "test-rule",
		sendAfter: now.Add(-time.Second),
		extTagsCb: func() ([]string, bool) {
			cbCallCount++
			return nil, true // empty but retryable
		},
	}
	server.queue = []*pendingMsg{msg}

	server.dequeue(now, server.tryResolve)

	if cbCallCount != 1 {
		t.Fatalf("expected extTagsCb called once, got %d", cbCallCount)
	}
	// message must be kept for retry
	if len(server.queue) != 1 {
		t.Fatalf("expected message to be kept in queue for retry, got size %d", len(server.queue))
	}
	if msg.retry != 1 {
		t.Fatalf("expected retry counter incremented to 1, got %d", msg.retry)
	}
	// a retry was scheduled → retryCount incremented; nothing sent so no missing-tags/skipped-retry
	if v := server.retryCount.Load(); v != 1 {
		t.Fatalf("expected retryCount=1, got %d", v)
	}
	if v := server.missingTagsCount.Load(); v != 0 {
		t.Fatalf("expected missingTagsCount=0 (message not yet sent), got %d", v)
	}
	if v := server.skippedRetryCount.Load(); v != 0 {
		t.Fatalf("expected skippedRetryCount=0, got %d", v)
	}
}

func TestExtTagsCbNoRetryWhenQueueFull(t *testing.T) {
	// When extTagsCb returns empty tags + retryable=true but isRetryAllowed=false → send anyway
	server := newTestAPIServer(1) // threshold=1 → queue full

	now := time.Now()
	cbCallCount := 0
	msg := &pendingMsg{
		ruleID:    "test-rule",
		sendAfter: now.Add(-time.Second),
		extTagsCb: func() ([]string, bool) {
			cbCallCount++
			return nil, true // empty but retryable — ignored because queue is full
		},
	}
	server.queue = []*pendingMsg{msg}

	server.dequeue(now, server.tryResolve)

	if cbCallCount != 1 {
		t.Fatalf("expected extTagsCb called once, got %d", cbCallCount)
	}
	// message must be sent despite empty tags because queue is full
	if len(server.queue) != 0 {
		t.Fatalf("expected message to be sent (empty queue), got size %d", len(server.queue))
	}
	// the forced send counts as one skipped retry and one missing-tags event
	if v := server.skippedRetryCount.Load(); v != 1 {
		t.Fatalf("expected skippedRetryCount=1, got %d", v)
	}
	if v := server.missingTagsCount.Load(); v != 1 {
		t.Fatalf("expected missingTagsCount=1, got %d", v)
	}
}

func TestExtTagsCbNoRetryWhenNotRetryable(t *testing.T) {
	// When extTagsCb returns empty tags + retryable=false → send anyway regardless of queue state
	server := newTestAPIServer(100) // retry would normally be allowed

	now := time.Now()
	msg := &pendingMsg{
		ruleID:    "test-rule",
		sendAfter: now.Add(-time.Second),
		extTagsCb: func() ([]string, bool) {
			return nil, false // not retryable
		},
	}
	server.queue = []*pendingMsg{msg}

	server.dequeue(now, server.tryResolve)

	if len(server.queue) != 0 {
		t.Fatalf("expected message to be sent when not retryable, got queue size %d", len(server.queue))
	}
	// not retryable → missing tags counted, but no skipped retry
	if v := server.missingTagsCount.Load(); v != 1 {
		t.Fatalf("expected missingTagsCount=1, got %d", v)
	}
	if v := server.skippedRetryCount.Load(); v != 0 {
		t.Fatalf("expected skippedRetryCount=0 (not retryable, not a skipped retry), got %d", v)
	}
}
