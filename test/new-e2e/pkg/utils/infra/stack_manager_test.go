// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package infra implements utilities to interact with a Pulumi infrastructure
package infra

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockWriter struct {
	logs []string
}

var _ io.Writer = &mockWriter{}

func (m *mockWriter) Write(p []byte) (n int, err error) {
	m.logs = append(m.logs, string(p))
	return 0, nil
}

type mockDatadogEventSender struct {
	events []datadogV1.EventCreateRequest
}

var _ datadogEventSender = &mockDatadogEventSender{}

func (m *mockDatadogEventSender) SendEvent(body datadogV1.EventCreateRequest) {
	m.events = append(m.events, body)
}

func TestStackManager(t *testing.T) {
	stackManager := GetStackManager()
	ctx := context.Background()

	t.Run("should-succeed-on-successful-run-function", func(t *testing.T) {
		t.Parallel()
		t.Log("Should succeed on successful run function")
		mockWriter := &mockWriter{
			logs: []string{},
		}
		mockDatadogEventSender := &mockDatadogEventSender{
			events: []datadogV1.EventCreateRequest{},
		}
		stackName := "test-successful"
		stack, result, err := stackManager.GetStackNoDeleteOnFailure(ctx, stackName, nil, func(ctx *pulumi.Context) error {
			return nil
		}, false, mockWriter, mockDatadogEventSender)
		require.NoError(t, err)
		require.NotNil(t, stack)
		defer func() {
			err := stackManager.DeleteStack(ctx, stackName, mockWriter)
			require.NoError(t, err)
		}()
		require.NotNil(t, result)
		assert.Len(t, mockWriter.logs, 0)
		assert.Len(t, mockDatadogEventSender.events, 1)
		assert.Contains(t, mockDatadogEventSender.events[0].Title, fmt.Sprintf("[E2E] Stack %s : success on Pulumi stack up", stackName))
	})

	for errCount := 0; errCount < stackUpMaxRetry; errCount++ {
		errCount := errCount
		t.Run(fmt.Sprintf("should-retry-and-succeed-%d", errCount), func(t *testing.T) {
			t.Parallel()
			t.Log("Should retry on failing run function and eventually succeed")
			mockWriter := &mockWriter{
				logs: []string{},
			}
			mockDatadogEventSender := &mockDatadogEventSender{
				events: []datadogV1.EventCreateRequest{},
			}
			stackUpCounter := 0
			stackName := fmt.Sprintf("test-retry-%d", errCount)
			stack, result, err := stackManager.GetStackNoDeleteOnFailure(ctx, stackName, nil, func(ctx *pulumi.Context) error {
				stackUpCounter++
				if stackUpCounter > errCount {
					return nil
				}
				return fmt.Errorf("error %d", stackUpCounter)
			}, false, mockWriter, mockDatadogEventSender)
			require.NoError(t, err)
			require.NotNil(t, stack)
			defer func() {
				err := stackManager.DeleteStack(ctx, stackName, mockWriter)
				require.NoError(t, err)
			}()
			require.NotNil(t, result)
			assert.Len(t, mockWriter.logs, errCount)
			for i := 0; i < errCount; i++ {
				assert.Contains(t, mockWriter.logs[i], "Retrying stack on error during stack up")
				assert.Contains(t, mockWriter.logs[i], fmt.Sprintf("error %d", i+1))
			}
			assert.Len(t, mockDatadogEventSender.events, errCount+1)
			for i := 0; i < errCount; i++ {
				assert.Contains(t, mockDatadogEventSender.events[i].Title, fmt.Sprintf("[E2E] Stack %s : error on Pulumi stack up", stackName))
			}
			assert.Contains(t, mockDatadogEventSender.events[len(mockDatadogEventSender.events)-1].Title, fmt.Sprintf("[E2E] Stack %s : success on Pulumi stack up", stackName))
		})
	}

	t.Run("should-eventually-fail", func(t *testing.T) {
		t.Parallel()
		t.Log("Should retry on failing run function and eventually fail")
		mockWriter := &mockWriter{
			logs: []string{},
		}
		mockDatadogEventSender := &mockDatadogEventSender{
			events: []datadogV1.EventCreateRequest{},
		}
		stackUpCounter := 0
		stackName := "test-retry-failure"
		stack, result, err := stackManager.GetStackNoDeleteOnFailure(ctx, stackName, nil, func(ctx *pulumi.Context) error {
			stackUpCounter++
			return fmt.Errorf("error %d", stackUpCounter)
		}, false, mockWriter, mockDatadogEventSender)
		assert.Error(t, err)
		assert.ErrorIs(t, err, internalError{}, "should be an internal error")
		require.NotNil(t, stack)
		defer func() {
			err := stackManager.DeleteStack(ctx, stackName, mockWriter)
			require.NoError(t, err)
		}()
		assert.Equal(t, auto.UpResult{}, result)
		assert.Len(t, mockWriter.logs, stackUpMaxRetry+1, fmt.Sprintf("should have %d logs", stackUpMaxRetry+1))
		for i := 0; i < stackUpMaxRetry; i++ {
			assert.Contains(t, mockWriter.logs[i], "Retrying stack on error during stack up")
			assert.Contains(t, mockWriter.logs[i], fmt.Sprintf("error %d", i+1))
		}
		assert.Len(t, mockDatadogEventSender.events, stackUpMaxRetry+1, fmt.Sprintf("should have %d events", stackUpMaxRetry+1))
		for i := 0; i < stackUpMaxRetry+1; i++ {
			assert.Contains(t, mockDatadogEventSender.events[i].Title, fmt.Sprintf("[E2E] Stack %s : error on Pulumi stack up", stackName))
		}
		assert.Contains(t, mockDatadogEventSender.events[len(mockDatadogEventSender.events)-1].Tags, "retry:NoRetry")
	})
}
