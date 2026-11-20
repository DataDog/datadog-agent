// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logging

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil/lifecycle"
)

type recordingLogger struct {
	events []fxevent.Event
}

func (l *recordingLogger) LogEvent(e fxevent.Event) { l.events = append(l.events, e) }

type debugLogger struct{ lines []string }

func (l *debugLogger) Debug(v ...interface{}) {
	for _, s := range v {
		l.lines = append(l.lines, s.(string))
	}
}

func TestLifecycleLoggerRecordsComponents(t *testing.T) {
	recorder := lifecycle.NewRecorder(time.Now(), time.Now)
	next := &recordingLogger{}
	logger := withLifecycleRecorder(next, recorder)

	logger.LogEvent(&fxevent.Run{Name: "github.com/DataDog/datadog-agent/comp/core/config.newConfig()", Kind: "provide", Runtime: time.Second})
	logger.LogEvent(&fxevent.OnStartExecuted{FunctionName: "github.com/DataDog/datadog-agent/comp/core/log.start()", Runtime: 2 * time.Second})
	logger.LogEvent(&fxevent.OnStartExecuted{FunctionName: "fast()", Runtime: time.Millisecond})
	assert.Len(t, next.events, 3, "events are forwarded")

	var startup lifecycle.Startup
	recorder.OnReady(func(s lifecycle.Startup) { startup = s })
	recorder.MarkReady()
	assert.Equal(t, []lifecycle.Timing{
		{Name: "comp/core/log.start()", Stage: lifecycle.StageOnStart, Seconds: 2},
		{Name: "comp/core/config.newConfig()", Stage: lifecycle.StageConstruct, Seconds: 1},
	}, startup.SlowComponents)
}

func TestEnableFxLoggingOnDebugKeepsRecording(t *testing.T) {
	var fxLogger fxevent.Logger
	dbg := &debugLogger{}
	app := fx.New(
		DefaultFxLoggingOption(),
		fx.Supply(dbg),
		EnableFxLoggingOnDebug[*debugLogger](),
		fx.Invoke(func(l fxevent.Logger) { fxLogger = l }),
	)
	require.NoError(t, app.Err())

	_, ok := fxLogger.(*LifecycleLogger)
	assert.True(t, ok, "the decorated logger still records the lifecycle events")
	_, ok = TracingLogger(fxLogger)
	assert.False(t, ok, "Fx tracing is disabled by default")
}
