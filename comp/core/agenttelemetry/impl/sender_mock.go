// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package agenttelemetryimpl

import (
	"context"
	"sync"
)

// senderMock is a mock implementation of sender.
type senderMock struct {
	sentMetrics []*agentmetric

	// Captures from the errortracking flush path. Protected by mu
	// because the flush job may run concurrently with test setup and
	// assertions; readers MUST take the lock or use a synchronisation
	// barrier (e.g. wait on runner.stop().Done) that establishes
	// happens-before with the job's completion.
	//
	// sendLogsCallCount counts sendLogsBatch invocations; sentLogs
	// flattens every batch into one accumulating slice. The pair lets
	// tests distinguish "1 call with N records" from "N calls with 1
	// record each" — the latter would be a regression to per-batch
	// dispatch that the flattened slice alone cannot detect.
	sentLogsMu        sync.Mutex
	sentLogs          []Log
	sendLogsCallCount int
}

func (s *senderMock) startSession(_ context.Context) *senderSession {
	return &senderSession{}
}
func (s *senderMock) flushSession(_ *senderSession) error {
	return nil
}
func (s *senderMock) sendAgentMetricPayloads(_ *senderSession, metrics []*agentmetric) {
	s.sentMetrics = append(s.sentMetrics, metrics...)
}
func (s *senderMock) sendEventPayload(_ *senderSession, _ *Event, _ map[string]interface{}) {
}
func (s *senderMock) sendLogsBatch(_ context.Context, logs []Log) error {
	s.sentLogsMu.Lock()
	defer s.sentLogsMu.Unlock()
	s.sendLogsCallCount++
	s.sentLogs = append(s.sentLogs, logs...)
	return nil
}

// capturedLogs returns a thread-safe snapshot of the records captured
// via sendLogsBatch. Tests should call this rather than reading
// sentLogs directly.
func (s *senderMock) capturedLogs() []Log {
	s.sentLogsMu.Lock()
	defer s.sentLogsMu.Unlock()
	out := make([]Log, len(s.sentLogs))
	copy(out, s.sentLogs)
	return out
}

// sendLogsCalls returns a thread-safe snapshot of how many times
// sendLogsBatch was invoked. Pair with capturedLogs to assert
// "one HTTP call per flush" (N records via 1 call, not 1 record via N
// calls).
func (s *senderMock) sendLogsCalls() int {
	s.sentLogsMu.Lock()
	defer s.sentLogsMu.Unlock()
	return s.sendLogsCallCount
}
