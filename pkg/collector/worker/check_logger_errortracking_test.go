// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package worker

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
	logslog "github.com/DataDog/datadog-agent/pkg/util/log/slog"
)

// stackError is an error carrying a stack for error tracking.
type stackError struct {
	stack errortracking.ExternalStack
}

func (e *stackError) Error() string                              { return "check failed" }
func (e *stackError) ExternalStack() errortracking.ExternalStack { return e.stack }

// captureErrorLogs routes the agent logger through an errortracking handler
// and returns the records it submits.
func captureErrorLogs(t *testing.T) func() []errortracking.ErrorLog {
	t.Helper()
	var mu sync.Mutex
	var logs []errortracking.ErrorLog
	submit := errortracking.Submitter(func(e errortracking.ErrorLog) {
		mu.Lock()
		defer mu.Unlock()
		logs = append(logs, e)
	})
	bouncer := errortracking.NewBouncer(time.Hour, 0)
	handler := errortracking.NewHandler(func() errortracking.Submitter { return submit }).
		WithBouncerLoader(func() *errortracking.Bouncer { return bouncer })

	log.SetupLogger(logslog.NewWrapper(handler), "debug")
	t.Cleanup(func() { log.SetupLogger(log.Default(), "debug") })

	return func() []errortracking.ErrorLog {
		mu.Lock()
		defer mu.Unlock()
		return append([]errortracking.ErrorLog(nil), logs...)
	}
}

func TestCheckLoggerErrorUsesCheckStack(t *testing.T) {
	logs := captureErrorLogs(t)
	cl := &CheckLogger{Check: newTestCheck("redisdb:abc")}

	cl.Error(&stackError{stack: errortracking.ExternalStack{
		Frames:    "datadog_checks.redisdb.redisdb.check\n\tdatadog_checks/redisdb/redisdb.py:42 +0x0",
		ErrorKind: "ConnectionError",
		Source:    "redisdb",
	}})

	got := logs()
	require.Len(t, got, 1)
	assert.Equal(t, "ConnectionError", got[0].ErrorKind)
	assert.Equal(t, "redisdb", got[0].Integration)
	assert.Contains(t, got[0].Stack, "datadog_checks/redisdb/redisdb.py:42")
}

func TestCheckLoggerErrorWithoutStackKeepsGoStack(t *testing.T) {
	logs := captureErrorLogs(t)
	cl := &CheckLogger{Check: newTestCheck("redisdb:abc")}

	cl.Error(errors.New("plain failure"))

	got := logs()
	require.Len(t, got, 1)
	assert.Empty(t, got[0].Stack)
	assert.Empty(t, got[0].Integration)
	assert.NotZero(t, got[0].PCsLen)
}
