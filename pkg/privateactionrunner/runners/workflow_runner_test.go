// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package runners

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/require"

	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	opmstesting "github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms/testing"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"k8s.io/apimachinery/pkg/util/sets"
)

type blockingRCClient struct{}

func (b *blockingRCClient) Subscribe(_ string, _ func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
}

type testTaskVerifier struct{}

func (v testTaskVerifier) UnwrapTask(task *types.Task) (*types.Task, error) {
	return task, nil
}

func TestWorkflowRunnerStopBeforeKeysReadyDoesNotStartLoop(t *testing.T) {
	keysManager := taskverifier.NewKeyManager(&blockingRCClient{})
	dequeueCalled := make(chan struct{}, 1)
	opmsClient := &opmstesting.FakeOpmsClient{
		DequeueTaskFn: func(_ context.Context) (*types.Task, time.Duration, error) {
			dequeueCalled <- struct{}{}
			return nil, 0, nil
		},
	}
	cfg := &parconfig.Config{
		ActionsAllowlist: map[string]sets.Set[string]{},
		LoopInterval:     time.Millisecond,
		MetricsClient:    &statsd.NoOpClient{},
		RunnerPoolSize:   1,
	}

	runner, err := NewWorkflowRunner(cfg, keysManager, testTaskVerifier{}, opmsClient, nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, runner.Start(context.Background()))

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, runner.Stop(stopCtx))

	runner.mu.Lock()
	require.Nil(t, runner.taskLoop)
	runner.mu.Unlock()
	select {
	case <-dequeueCalled:
		require.Fail(t, "workflow loop started after runner was stopped")
	default:
	}
}
