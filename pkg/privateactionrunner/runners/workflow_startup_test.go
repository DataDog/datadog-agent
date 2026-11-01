// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runners

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/encryptioncontext"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

type readinessOPMS struct {
	opms.Client
	polled chan struct{}
}

func (c *readinessOPMS) DequeueTask(context.Context) (*types.Task, time.Duration, error) {
	select {
	case c.polled <- struct{}{}:
	default:
	}
	return nil, time.Hour, nil
}

func TestWorkflowStartupCancellation(t *testing.T) {
	for _, readyBeforeStop := range []bool{false, true} {
		t.Run(map[bool]string{false: "late_callback", true: "start_context_expires"}[readyBeforeStop], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				manager, callback := taskverifier.NewKeyManagerWithCallback()
				client := &readinessOPMS{polled: make(chan struct{}, 1)}
				runner := &WorkflowRunner{
					config:      &config.Config{MetricsClient: &statsd.NoOpClient{}, LoopInterval: time.Hour, MaxAttempts: 1},
					keysManager: manager, opmsClient: client,
					encryptionStore: encryptioncontext.NewStore(), shutdownChannel: make(chan struct{}),
				}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				require.NoError(t, runner.Start(ctx))
				synctest.Wait()
				cancel()
				if readyBeforeStop {
					callback(nil, func(string, state.ApplyStatus) {})
					<-client.polled
				}
				require.NoError(t, runner.Stop(t.Context()))
				callback(nil, func(string, state.ApplyStatus) {})
				synctest.Wait()
				select {
				case <-client.polled:
					t.Fatal("runner started polling after shutdown")
				default:
				}
			})
		})
	}
}
