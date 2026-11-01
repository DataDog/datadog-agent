// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package privateactionrunnerimpl

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/executor"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

func TestExecutorReadinessStopsBeforeCallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		manager, callback := taskverifier.NewKeyManagerWithCallback()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		runner := newStartedRunnerForStopTest(nil, false)
		runner.cancelStart = cancel
		runner.logger = logmock.New(t)
		runner.executorServer = executor.NewServer(nil, "test")
		runner.startExecutorReadiness(ctx, manager)
		synctest.Wait()
		require.NoError(t, runner.StopExecutor(t.Context()))
		callback(nil, func(string, state.ApplyStatus) {})
		synctest.Wait()
		health, err := runner.executorServer.Health(t.Context(), nil)
		require.NoError(t, err)
		require.False(t, health.Ready)
	})
}
