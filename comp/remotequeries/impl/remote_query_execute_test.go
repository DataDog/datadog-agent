// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeriesimpl

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

func singleMatchCollector(instance string) fakeCollector {
	return fakeCollector{checks: []check.Check{
		fakeWrappedCheck{Check: &fakeStreamRunnerCheck{
			fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: instance}},
			events:          []check.RemoteQueryStreamEvent{{Type: "final", MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":{"uploadId":"upload-proof","pageCount":1,"totalRows":1,"totalBytes":9}}`}},
		}},
	}}
}

func executeTestRequest(t *testing.T) RemoteQueryExecuteRequest {
	t.Helper()
	req, err := NewRemoteQueryExecuteRequest("postgres",
		RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"},
		remoteQueryFixtureTableProofQuery, false, pagedTestDelivery())
	require.NoError(t, err)
	return req
}

// TestExecuteStreamAnswersPlainMatchOutcomes proves execute resolves the target
// fresh under the execution admission with the plain zero/one/many outcomes of the
// sweep — there is no resolve-time binding: a unique match executes even if the
// check identity changed since any earlier resolve, a no-match sweep answers
// target_not_found, and multiple matched verdicts answer ambiguous_target before
// the execute dispatch, so no Python execution call and no SQL ever happens.
func TestExecuteStreamAnswersPlainMatchOutcomes(t *testing.T) {
	t.Run("unique match executes", func(t *testing.T) {
		collector := singleMatchCollector("host: localhost\nport: 5432\ndbname: postgres\npassword: secret-value\n")
		runner := collector.checks[0].(fakeWrappedCheck).Check.(*fakeStreamRunnerCheck)
		service := NewRemoteQueryExecuteService(collector, true, false, nil)

		result := service.ExecuteStream(context.Background(), executeTestRequest(t), func(check.RemoteQueryStreamEvent) error { return nil })

		require.Nil(t, result.Error)
		assert.Equal(t, http.StatusOK, result.HTTPStatus)
		assert.Equal(t, 1, runner.executeCalls)
		// The execute-time sweep asked the check once before the execute dispatch.
		assert.Equal(t, 1, runner.resolveCalls)
	})

	t.Run("zero matched verdicts answers target_not_found", func(t *testing.T) {
		collector := singleMatchCollector("host: localhost\nport: 5432\ndbname: postgres\n")
		runner := collector.checks[0].(fakeWrappedCheck).Check.(*fakeStreamRunnerCheck)
		runner.resolveEvents = resolveTargetNotFoundEvents()
		service := NewRemoteQueryExecuteService(collector, true, false, nil)

		result := service.ExecuteStream(context.Background(), executeTestRequest(t), func(check.RemoteQueryStreamEvent) error { return nil })

		require.NotNil(t, result.Error)
		assert.Equal(t, http.StatusNotFound, result.HTTPStatus)
		assert.Equal(t, statusTargetNotFound, result.Error.Code)
		assert.Zero(t, runner.executeCalls)
	})

	t.Run("multiple matched verdicts answers ambiguous_target", func(t *testing.T) {
		collector := fakeCollector{checks: []check.Check{
			fakeWrappedCheck{Check: &fakeStreamRunnerCheck{fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-one\n"}}}},
			fakeWrappedCheck{Check: &fakeStreamRunnerCheck{fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "kube", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-two\n"}}}},
		}}
		service := NewRemoteQueryExecuteService(collector, true, false, nil)

		result := service.ExecuteStream(context.Background(), executeTestRequest(t), func(check.RemoteQueryStreamEvent) error { return nil })

		require.NotNil(t, result.Error)
		assert.Equal(t, http.StatusConflict, result.HTTPStatus)
		assert.Equal(t, statusAmbiguous, result.Error.Code)
		for _, chk := range collector.checks {
			runner := chk.(fakeWrappedCheck).Check.(*fakeStreamRunnerCheck)
			assert.Zero(t, runner.executeCalls)
		}
	})
}
