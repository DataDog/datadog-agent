// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package runners

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/encryptioncontext"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type blockingKeysManager struct {
	waitStarted chan struct{}
	waitDone    chan error
}

func (m *blockingKeysManager) Start(context.Context)          {}
func (m *blockingKeysManager) GetKey(string) types.DecodedKey { return nil }
func (m *blockingKeysManager) Seed([]taskverifier.SigningKey) error {
	return nil
}
func (m *blockingKeysManager) Snapshot() []taskverifier.SigningKey { return nil }
func (m *blockingKeysManager) WaitForReady(ctx context.Context) error {
	close(m.waitStarted)
	<-ctx.Done()
	err := ctx.Err()
	m.waitDone <- err
	return err
}

func TestWorkflowRunnerStartWaitsForKeysBeyondStartDeadline(t *testing.T) {
	keysManager := &blockingKeysManager{
		waitStarted: make(chan struct{}),
		waitDone:    make(chan error, 1),
	}
	runner := &WorkflowRunner{
		config:          &config.Config{MetricsClient: &statsd.NoOpClient{}},
		keysManager:     keysManager,
		encryptionStore: encryptioncontext.NewStore(),
		shutdownChannel: make(chan struct{}),
	}

	startCtx, cancelStart := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancelStart()
	require.NoError(t, runner.Start(startCtx))
	<-keysManager.waitStarted
	<-startCtx.Done()

	select {
	case err := <-keysManager.waitDone:
		require.Failf(t, "key wait stopped at the Fx start deadline", "error: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	stopCtx, cancelStop := context.WithTimeout(context.Background(), time.Second)
	defer cancelStop()
	require.NoError(t, runner.Stop(stopCtx))
	require.ErrorIs(t, <-keysManager.waitDone, context.Canceled)
}
