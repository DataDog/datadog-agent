// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package infra implements utilities to interact with a Pulumi infrastructure
package infra

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/common"

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
		stack, result, err := stackManager.GetStackNoDeleteOnFailure(
			ctx,
			stackName,
			func(*pulumi.Context) error {
				return nil
			},
			WithLogWriter(mockWriter),
			WithDatadogEventSender(mockDatadogEventSender),
		)
		require.NoError(t, err)
		require.NotNil(t, stack)
		defer func() {
			err := stackManager.DeleteStack(ctx, stackName, mockWriter)
			require.NoError(t, err)
		}()
		require.NotNil(t, result)
		retryOnErrorLogs := filterRetryOnErrorLogs(mockWriter.logs)
		assert.Empty(t, retryOnErrorLogs)
		assert.Len(t, mockDatadogEventSender.events, 1)
		assert.Contains(t, mockDatadogEventSender.events[0].Title, fmt.Sprintf("[E2E] Stack %s : success on Pulumi stack up", stackName))
	})

	t.Run("should-retry-and-succeed", func(t *testing.T) {
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
				stack, result, err := stackManager.GetStackNoDeleteOnFailure(
					ctx,
					stackName,
					func(*pulumi.Context) error {
						stackUpCounter++
						if stackUpCounter > errCount {
							return nil
						}
						return fmt.Errorf("error %d", stackUpCounter)
					},
					WithLogWriter(mockWriter),
					WithDatadogEventSender(mockDatadogEventSender),
				)
				require.NoError(t, err)
				require.NotNil(t, stack)
				defer func() {
					err := stackManager.DeleteStack(ctx, stackName, mockWriter)
					require.NoError(t, err)
				}()
				require.NotNil(t, result)
				retryOnErrorLogs := filterRetryOnErrorLogs(mockWriter.logs)
				assert.Len(t, retryOnErrorLogs, errCount, fmt.Sprintf("should have %d error logs", errCount))
				for i := 0; i < errCount; i++ {
					assert.Contains(t, retryOnErrorLogs[i], "Retrying stack on error during stack up")
					assert.Contains(t, retryOnErrorLogs[i], fmt.Sprintf("error %d", i+1))
				}
				assert.Len(t, mockDatadogEventSender.events, errCount+1)
				for i := 0; i < errCount; i++ {
					assert.Contains(t, mockDatadogEventSender.events[i].Title, fmt.Sprintf("[E2E] Stack %s : error on Pulumi stack up", stackName))
				}
				assert.Contains(t, mockDatadogEventSender.events[len(mockDatadogEventSender.events)-1].Title, fmt.Sprintf("[E2E] Stack %s : success on Pulumi stack up", stackName))
			})
		}
	})

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
		stack, result, err := stackManager.GetStackNoDeleteOnFailure(
			ctx,
			stackName,
			func(*pulumi.Context) error {
				stackUpCounter++
				return fmt.Errorf("error %d", stackUpCounter)
			},
			WithLogWriter(mockWriter),
			WithDatadogEventSender(mockDatadogEventSender),
		)
		assert.Error(t, err)
		assert.ErrorIs(t, err, common.InternalError{}, "should be an internal error")
		require.NotNil(t, stack)
		defer func() {
			err := stackManager.DeleteStack(ctx, stackName, mockWriter)
			require.NoError(t, err)
		}()
		assert.Equal(t, auto.UpResult{}, result)

		retryOnErrorLogs := filterRetryOnErrorLogs(mockWriter.logs)
		assert.Len(t, retryOnErrorLogs, stackUpMaxRetry, fmt.Sprintf("should have %d logs", stackUpMaxRetry+1))
		for i := 0; i < stackUpMaxRetry; i++ {
			assert.Contains(t, retryOnErrorLogs[i], "Retrying stack on error during stack up")
			assert.Contains(t, retryOnErrorLogs[i], fmt.Sprintf("error %d", i+1))
		}
		assert.Len(t, mockDatadogEventSender.events, stackUpMaxRetry+1, fmt.Sprintf("should have %d events", stackUpMaxRetry+1))
		for i := 0; i < stackUpMaxRetry+1; i++ {
			assert.Contains(t, mockDatadogEventSender.events[i].Title, fmt.Sprintf("[E2E] Stack %s : error on Pulumi stack up", stackName))
		}
		assert.Contains(t, mockDatadogEventSender.events[len(mockDatadogEventSender.events)-1].Tags, "retry:NoRetry")
	})

	t.Run("should-cancel-and-retry-on-timeout", func(t *testing.T) {
		t.Parallel()

		mockWriter := &mockWriter{
			logs: []string{},
		}
		mockDatadogEventSender := &mockDatadogEventSender{
			events: []datadogV1.EventCreateRequest{},
		}
		stackUpCounter := 0
		stackName := "test-cancel-retry-timeout"
		// override stackUpTimeout to 10s
		// average up time with an dummy run function is 5s
		stackUpTimeout := 10 * time.Second
		stack, result, err := stackManager.GetStackNoDeleteOnFailure(
			ctx,
			stackName,
			func(*pulumi.Context) error {
				if stackUpCounter == 0 {
					// sleep only first time to ensure context is cancelled
					// on timeout
					t.Logf("Sleeping for %f", 2*stackUpTimeout.Seconds())
					time.Sleep(2 * stackUpTimeout)
				}
				stackUpCounter++
				return nil
			},
			WithLogWriter(mockWriter),
			WithDatadogEventSender(mockDatadogEventSender),
			WithUpTimeout(stackUpTimeout),
		)

		assert.NoError(t, err)
		require.NotNil(t, stack)
		assert.NotNil(t, result)
		defer func() {
			err := stackManager.DeleteStack(ctx, stackName, mockWriter)
			require.NoError(t, err)
		}()
		// filter timeout logs
		timeoutLogs := []string{}
		for _, log := range mockWriter.logs {
			if strings.Contains(log, "Timeout during stack up, trying to cancel stack's operation") {
				timeoutLogs = append(timeoutLogs, log)
			}
		}
		assert.Len(t, timeoutLogs, 1)
		retryOnErrorLogs := filterRetryOnErrorLogs(mockWriter.logs)
		assert.Len(t, retryOnErrorLogs, 1)
		assert.Len(t, mockDatadogEventSender.events, 3)
		assert.Contains(t, mockDatadogEventSender.events[0].Title, fmt.Sprintf("[E2E] Stack %s : timeout on Pulumi stack up", stackName))
		assert.Contains(t, mockDatadogEventSender.events[1].Title, fmt.Sprintf("[E2E] Stack %s : error on Pulumi stack up", stackName))
		assert.Contains(t, mockDatadogEventSender.events[2].Title, fmt.Sprintf("[E2E] Stack %s : success on Pulumi stack up", stackName))
	})

	t.Run("should-return-retry-strategy-on-retriable-errors", func(t *testing.T) {
		t.Parallel()

		type testError struct {
			name              string
			errMessage        string
			expectedRetryType RetryType
		}

		testErrors := []testError{
			{
				name:              "timeout",
				errMessage:        "i/o timeout",
				expectedRetryType: ReCreate,
			},
			{
				name:              "connection-refused",
				errMessage:        "failed attempts: dial tcp :22: connect: connection refused",
				expectedRetryType: ReCreate,
			},
			{
				name:              "resource-not-exist",
				errMessage:        "Resource provider reported that the resource did not exist while updating",
				expectedRetryType: ReCreate,
			},
			{
				name:              "cloud-init-timeout",
				errMessage:        "Process exited with status 2: running \" sudo cloud-init status --wait\"",
				expectedRetryType: ReCreate,
			},
			{
				name:              "cloud-init-timeout",
				errMessage:        "Process exited with status 6: running \" sudo cloud-init status --wait\"",
				expectedRetryType: ReCreate,
			},
			{
				name:              "ecs-fakeintake-timeout",
				errMessage:        "waiting for ECS Service (arn:aws:ecs:us-east-1:669783387624:service/fakeintake-ecs/ci-633219896-4670-e2e-dockersuite-80f62edf7bcc6194-aws-fakeintake-dockervm-srv) create: timeout while waiting for state to become 'tfSTABLE' (last state: 'tfPENDING', timeout: 20m0s)",
				expectedRetryType: ReCreate,
			},
		}

		for _, te := range testErrors {
			err := errors.New(te.errMessage)
			retryType, _ := stackManager.getRetryStrategyFrom(err, 0)
			assert.Equal(t, te.expectedRetryType, retryType, te.name)
		}
	})
}

func filterRetryOnErrorLogs(logs []string) []string {
	retryOnErrorLogs := []string{}
	for _, log := range logs {
		if strings.Contains(log, "Retrying stack on error during stack up") {
			retryOnErrorLogs = append(retryOnErrorLogs, log)
		}
	}
	return retryOnErrorLogs
}
