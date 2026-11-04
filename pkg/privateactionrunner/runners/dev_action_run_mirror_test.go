// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package runners

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/mocktracer"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	privatebundles "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/devtracing"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/observability"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func TestMain(m *testing.M) {
	devtracing.DisableTracerStartForTest()
	os.Exit(m.Run())
}

type mirrorTestAction struct {
	ctx context.Context
	err error
}

func (a *mirrorTestAction) Run(ctx context.Context, _ *types.Task, _ *privateconnection.PrivateCredentials) (interface{}, error) {
	a.ctx = ctx
	return "ok", a.err
}

type mirrorTestBundle struct{ action types.Action }

func (b mirrorTestBundle) GetAction(string) types.Action { return b.action }

func runMirrorTask(t *testing.T, bundleID string, traceID, spanID uint64, action *mirrorTestAction) error {
	t.Helper()
	executor := &WorkflowTaskExecutor{
		registry: &privatebundles.Registry{Bundles: map[string]types.Bundle{bundleID: mirrorTestBundle{action}}},
		config: &config.Config{
			ActionsAllowlist: map[string]sets.Set[string]{bundleID: sets.New("execute")},
			MetricsClient:    &statsd.NoOpClient{},
		},
	}
	task := newWorkflowTask("task-1", bundleID, "execute", "job-1")
	task.Data.Attributes.TraceId = traceID
	task.Data.Attributes.SpanId = spanID
	_, err := executor.RunTask(context.Background(), &PreparedWorkflowTask{Task: task})
	return err
}

const (
	mirrorBundle  = "com.datadoghq.remoteaction.queries"
	mirrorTraceID = uint64(1234567890123456789)
	mirrorSpanID  = uint64(200)
)

func TestRunTaskMirrorsActionRunForRemoteQueries(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	action := &mirrorTestAction{err: errors.New("boom")}
	err := runMirrorTask(t, mirrorBundle, mirrorTraceID, mirrorSpanID, action)
	require.Error(t, err)

	// The mini-tracer span's own id is what the bundle reads from the context.
	var miniSpanID uint64
	for _, entry := range telemetry.EnvFromContext(action.ctx) {
		if value, ok := strings.CutPrefix(entry, "DATADOG_PARENT_ID="); ok {
			var perr error
			miniSpanID, perr = strconv.ParseUint(value, 10, 64)
			require.NoError(t, perr)
		}
	}
	require.NotZero(t, miniSpanID)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, observability.ActionRunOperation, span.OperationName())
	assert.Equal(t, miniSpanID, span.SpanID())
	assert.Equal(t, mirrorSpanID, span.ParentID())
	assert.Equal(t, mirrorTraceID, span.TraceID())
	assert.Equal(t, mirrorBundle+".execute", span.Tag("resource.name"))
	assert.Equal(t, "task-1", span.Tag("task_id"))
	assert.Equal(t, observability.ParService, span.Tag("service.name"))
	assert.Equal(t, float64(2), span.Tag("_sampling_priority_v1"))
	assert.NotNil(t, span.Tag("error.message"), "the action error must propagate to the mirror")
}

func TestRunTaskMirrorSuccessHasNoError(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	require.NoError(t, runMirrorTask(t, mirrorBundle, mirrorTraceID, mirrorSpanID, &mirrorTestAction{}))
	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	assert.Nil(t, spans[0].Tag("error.message"))
}

func TestRunTaskMirrorDoesNotChangeActionContext(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	action := &mirrorTestAction{}
	require.NoError(t, runMirrorTask(t, mirrorBundle, mirrorTraceID, mirrorSpanID, action))
	// The context still carries the mini-tracer identity and no dd-trace-go span.
	assert.NotEmpty(t, telemetry.EnvFromContext(action.ctx))
	_, ok := tracer.SpanFromContext(action.ctx)
	assert.False(t, ok)
}

func TestRunTaskNoMirrorWhenNotEligible(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		defer devtracing.SetEnabledForTest(false)()
		mt := mocktracer.Start()
		defer mt.Stop()
		require.NoError(t, runMirrorTask(t, mirrorBundle, mirrorTraceID, mirrorSpanID, &mirrorTestAction{}))
		assert.Empty(t, mt.FinishedSpans())
	})
	t.Run("other action", func(t *testing.T) {
		mt := mocktracer.Start()
		defer mt.Stop()
		require.NoError(t, runMirrorTask(t, "com.datadoghq.other", mirrorTraceID, mirrorSpanID, &mirrorTestAction{}))
		assert.Empty(t, mt.FinishedSpans())
	})
	t.Run("zero trace id", func(t *testing.T) {
		mt := mocktracer.Start()
		defer mt.Stop()
		require.NoError(t, runMirrorTask(t, mirrorBundle, 0, mirrorSpanID, &mirrorTestAction{}))
		assert.Empty(t, mt.FinishedSpans())
	})
}
