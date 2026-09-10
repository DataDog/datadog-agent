// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeriesimpl

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// resolveTestRunner builds one runner-backed postgres check whose resolver answers
// the configured verdict; the instance config stays on the fake only to prove the
// sweep never surfaces it.
func resolveTestRunner(provider string, resolveEvents []check.RemoteQueryStreamEvent) *fakeStreamRunnerCheck {
	return &fakeStreamRunnerCheck{
		fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{
			name:     "postgres",
			loader:   "python",
			provider: provider,
			instance: "host: localhost\nport: 5432\ndbname: postgres\nusername: alice\npassword: secret-value\n",
		}},
		resolveEvents: resolveEvents,
	}
}

func resolveTestCollector() fakeCollector {
	return fakeCollector{checks: []check.Check{
		fakeWrappedCheck{Check: resolveTestRunner("file", nil)},
		fakeWrappedCheck{Check: resolveTestRunner("kube", resolveTargetNotFoundEvents())},
		fakeCheck{name: "mysql", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: mysql-secret\n"},
	}}
}

func resolveTupleRequest() RemoteQueryResolveRequest {
	return RemoteQueryResolveRequest{
		Integration: "postgres",
		Target:      RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"},
	}
}

// TestRemoteQueryResolveServiceAnswersStructuredOutcomes proves the resolve
// service reduces the integration-owned sweep to the match-before-execute
// statuses: all no-match verdicts answer target_not_found, exactly one matched
// verdict answers matched with a non-empty fingerprint, and more than one matched
// verdict answers ambiguous_target. No secret from any instance config or verdict
// metadata surfaces in any result.
func TestRemoteQueryResolveServiceAnswersStructuredOutcomes(t *testing.T) {
	service := NewRemoteQueryResolveService(resolveTestCollector(), true)

	t.Run("zero matched verdicts", func(t *testing.T) {
		noneService := NewRemoteQueryResolveService(fakeCollector{checks: []check.Check{
			fakeWrappedCheck{Check: resolveTestRunner("file", resolveTargetNotFoundEvents())},
			fakeWrappedCheck{Check: resolveTestRunner("kube", resolveTargetNotFoundEvents())},
		}}, true)

		result := noneService.Resolve(RemoteQueryResolveRequest{
			Integration: "postgres",
			Target:      RemoteQueryExecuteTarget{Host: "nowhere", Port: 5432, DBName: "other"},
		})

		assert.Equal(t, http.StatusNotFound, result.HTTPStatus)
		assert.Equal(t, statusTargetNotFound, result.Status)
		require.NotNil(t, result.Error)
		assert.Equal(t, statusTargetNotFound, result.Error.Code)
		assert.Equal(t, "no matching integration check found", result.Error.Message)
		assert.Empty(t, result.MatchFingerprint)
		assert.NotContains(t, result.Error.Message, "secret-value")
		assert.NotContains(t, result.Error.Message, "other-secret")
	})

	t.Run("zero loaded checks of the integration", func(t *testing.T) {
		emptyService := NewRemoteQueryResolveService(fakeCollector{checks: []check.Check{
			fakeCheck{name: "mysql", loader: "python", provider: "file", instance: "host: localhost\nport: 3306\ndbname: mysql\npassword: mysql-secret\n"},
		}}, true)

		result := emptyService.Resolve(resolveTupleRequest())

		assert.Equal(t, http.StatusNotFound, result.HTTPStatus)
		assert.Equal(t, statusTargetNotFound, result.Status)
		assert.NotContains(t, result.Error.Message, "mysql-secret")
	})

	t.Run("exactly one matched verdict", func(t *testing.T) {
		result := service.Resolve(resolveTupleRequest())

		assert.Equal(t, http.StatusOK, result.HTTPStatus)
		assert.Equal(t, statusMatched, result.Status)
		assert.Nil(t, result.Error)
		assert.Regexp(t, fingerprintHexPattern, result.MatchFingerprint)
	})

	// The resolver owns database eligibility: a requested database the raw instance
	// config does not name is a live match when the integration's eligible set
	// (an autodiscovered database, or the materialized effective default) admits
	// it. The Go side never compares the requested dbname against the raw YAML.
	t.Run("integration-admitted database is matched", func(t *testing.T) {
		result := service.Resolve(RemoteQueryResolveRequest{
			Integration: "postgres",
			Target:      RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "autodiscovered_ok"},
		})

		assert.Equal(t, http.StatusOK, result.HTTPStatus)
		assert.Equal(t, statusMatched, result.Status)
		assert.Nil(t, result.Error)
		assert.Regexp(t, fingerprintHexPattern, result.MatchFingerprint)
	})

	t.Run("multiple matched verdicts", func(t *testing.T) {
		duplicateService := NewRemoteQueryResolveService(fakeCollector{checks: []check.Check{
			fakeWrappedCheck{Check: resolveTestRunner("file", nil)},
			fakeWrappedCheck{Check: resolveTestRunner("kube", nil)},
		}}, true)

		result := duplicateService.Resolve(RemoteQueryResolveRequest{
			Integration: "postgres",
			Target:      RemoteQueryExecuteTarget{DatabaseInstance: "duplicate"},
		})

		assert.Equal(t, http.StatusConflict, result.HTTPStatus)
		assert.Equal(t, statusAmbiguous, result.Status)
		require.NotNil(t, result.Error)
		assert.Equal(t, statusAmbiguous, result.Error.Code)
		assert.Empty(t, result.MatchFingerprint)
	})
}

// TestRemoteQueryResolveServiceDatabaseInstanceTarget proves the managed-instance
// selector is delegated to the resolver sweep like tuple matching: the matched
// check's verdict decides, and the fingerprint binds the integration-reported
// identity.
func TestRemoteQueryResolveServiceDatabaseInstanceTarget(t *testing.T) {
	service := NewRemoteQueryResolveService(fakeCollector{checks: []check.Check{
		fakeWrappedCheck{Check: resolveTestRunner("file", nil)},
	}}, true)

	result := service.Resolve(RemoteQueryResolveRequest{
		Integration: "postgres",
		Target:      RemoteQueryExecuteTarget{DatabaseInstance: "rq-proof-a1-db1"},
	})

	assert.Equal(t, http.StatusOK, result.HTTPStatus)
	assert.Equal(t, statusMatched, result.Status)
	assert.Regexp(t, fingerprintHexPattern, result.MatchFingerprint)
}

// TestRemoteQueryResolveServiceSweepFailuresAreResolutionErrors proves every
// inability to establish the eligible set fails the aggregate resolution —
// never a silent target miss: a loaded check without the bridge resolver, a
// failed bridge call, an invalid verdict, and a busy admission all answer
// resolution_error with sanitized messages.
func TestRemoteQueryResolveServiceSweepFailuresAreResolutionErrors(t *testing.T) {
	t.Run("loaded check without the bridge resolver", func(t *testing.T) {
		service := NewRemoteQueryResolveService(fakeCollector{checks: []check.Check{
			fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-value\n"},
		}}, true)

		result := service.Resolve(resolveTupleRequest())

		assert.Equal(t, http.StatusFailedDependency, result.HTTPStatus)
		assert.Equal(t, statusResolutionError, result.Status)
		require.NotNil(t, result.Error)
		assert.Equal(t, "loaded integration check does not support remote query resolution", result.Error.Message)
		assert.NotContains(t, result.Error.Message, "secret-value")
	})

	t.Run("failed bridge call", func(t *testing.T) {
		service := NewRemoteQueryResolveService(fakeCollector{checks: []check.Check{
			fakeWrappedCheck{Check: &fakeStreamRunnerCheck{
				fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-value\n"}},
				resolveErr:      assert.AnError,
			}},
		}}, true)

		result := service.Resolve(resolveTupleRequest())

		assert.Equal(t, http.StatusFailedDependency, result.HTTPStatus)
		assert.Equal(t, statusResolutionError, result.Status)
		assert.Equal(t, "remote query resolver bridge call failed", result.Error.Message)
		assert.NotContains(t, result.Error.Message, assert.AnError.Error())
	})

	t.Run("invalid verdict", func(t *testing.T) {
		service := NewRemoteQueryResolveService(fakeCollector{checks: []check.Check{
			fakeWrappedCheck{Check: resolveTestRunner("file", []check.RemoteQueryStreamEvent{
				{Type: "final", MetadataJSON: `{"status":"MATCHED","match":{"host":"localhost","port":5432}}`},
			})},
		}}, true)

		result := service.Resolve(resolveTupleRequest())

		assert.Equal(t, http.StatusFailedDependency, result.HTTPStatus)
		assert.Equal(t, statusResolutionError, result.Status)
		assert.Equal(t, "remote query resolver returned an invalid verdict", result.Error.Message)
	})

	t.Run("busy admission fails fast", func(t *testing.T) {
		service := NewRemoteQueryResolveService(resolveTestCollector(), true)

		remoteQueryExecution.Lock()
		defer remoteQueryExecution.Unlock()

		result := service.Resolve(resolveTupleRequest())

		assert.Equal(t, http.StatusServiceUnavailable, result.HTTPStatus)
		assert.Equal(t, statusResolutionError, result.Status)
		require.NotNil(t, result.Error)
		assert.Equal(t, "another remote query is running on this Agent", result.Error.Message)
		assert.Empty(t, result.MatchFingerprint)
	})
}

// TestRemoteQueryResolveServiceFailuresAreResolutionErrors proves every failure
// to even start resolving answers resolution_error — never a silent target miss: a
// disabled bridge, a missing collector, and malformed input are all internal or
// contract errors with sanitized messages.
func TestRemoteQueryResolveServiceFailuresAreResolutionErrors(t *testing.T) {
	t.Run("disabled bridge", func(t *testing.T) {
		service := NewRemoteQueryResolveService(resolveTestCollector(), false)

		result := service.Resolve(resolveTupleRequest())

		assert.Equal(t, http.StatusServiceUnavailable, result.HTTPStatus)
		assert.Equal(t, statusResolutionError, result.Status)
		require.NotNil(t, result.Error)
		assert.Equal(t, statusResolutionError, result.Error.Code)
		assert.Equal(t, "remote queries resolve bridge is disabled", result.Error.Message)
	})

	t.Run("nil service", func(t *testing.T) {
		var service *RemoteQueryResolveService

		result := service.Resolve(resolveTupleRequest())

		assert.Equal(t, http.StatusServiceUnavailable, result.HTTPStatus)
		assert.Equal(t, statusResolutionError, result.Status)
	})

	t.Run("unavailable collector", func(t *testing.T) {
		service := NewRemoteQueryResolveService(nil, true)

		result := service.Resolve(resolveTupleRequest())

		assert.Equal(t, http.StatusFailedDependency, result.HTTPStatus)
		assert.Equal(t, statusResolutionError, result.Status)
		require.NotNil(t, result.Error)
		assert.Equal(t, "remote query resolver is unavailable", result.Error.Message)
	})

	t.Run("malformed integration", func(t *testing.T) {
		service := NewRemoteQueryResolveService(resolveTestCollector(), true)

		result := service.Resolve(RemoteQueryResolveRequest{
			Integration: "my-sql",
			Target:      RemoteQueryExecuteTarget{Host: "localhost", Port: 3306, DBName: "mysql"},
		})

		assert.Equal(t, http.StatusBadRequest, result.HTTPStatus)
		assert.Equal(t, statusResolutionError, result.Status)
		require.NotNil(t, result.Error)
		assert.Equal(t, "integration contains invalid characters", result.Error.Message)
	})

	t.Run("malformed target", func(t *testing.T) {
		service := NewRemoteQueryResolveService(resolveTestCollector(), true)

		result := service.Resolve(RemoteQueryResolveRequest{
			Integration: "postgres",
			Target:      RemoteQueryExecuteTarget{Host: "", Port: 5432, DBName: "postgres"},
		})

		assert.Equal(t, http.StatusBadRequest, result.HTTPStatus)
		assert.Equal(t, statusResolutionError, result.Status)
		require.NotNil(t, result.Error)
		assert.Equal(t, "target.host is required", result.Error.Message)
	})
}

// TestRemoteQueryResolveHandlerAnswersContractShape proves the HTTP diagnostic
// endpoint answers the resolve output contract: status always, matchFingerprint
// only when present, and the error object mirroring the status otherwise.
func TestRemoteQueryResolveHandlerAnswersContractShape(t *testing.T) {
	t.Run("matched", func(t *testing.T) {
		handler := &remoteQueryResolveHandler{service: NewRemoteQueryResolveService(resolveTestCollector(), true)}

		recorder := callResolveHandler(handler, `{"integration":"postgres","target":{"host":"LOCALHOST.","port":5432,"dbname":"postgres"}}`)

		assert.Equal(t, http.StatusOK, recorder.Code)
		body := recorder.Body.String()
		assert.Contains(t, body, `"status":"matched"`)
		assert.Contains(t, body, `"matchFingerprint":"`)
		assert.NotContains(t, body, `"error"`)
		assert.NotContains(t, body, "secret-value")
		assert.NotContains(t, body, "other-secret")
		assert.NotContains(t, body, "mysql-secret")
		assert.NotContains(t, body, "alice")
	})

	t.Run("target not found", func(t *testing.T) {
		handler := &remoteQueryResolveHandler{service: NewRemoteQueryResolveService(fakeCollector{checks: []check.Check{
			fakeWrappedCheck{Check: resolveTestRunner("file", resolveTargetNotFoundEvents())},
		}}, true)}

		recorder := callResolveHandler(handler, `{"integration":"postgres","target":{"host":"nowhere","port":5432,"dbname":"other"}}`)

		assert.Equal(t, http.StatusNotFound, recorder.Code)
		body := recorder.Body.String()
		assert.Contains(t, body, `"status":"target_not_found"`)
		assert.Contains(t, body, `"error":{"code":"target_not_found","message":"no matching integration check found"}`)
		assert.NotContains(t, body, `"matchFingerprint"`)
	})

	t.Run("ambiguous", func(t *testing.T) {
		handler := &remoteQueryResolveHandler{service: NewRemoteQueryResolveService(fakeCollector{checks: []check.Check{
			fakeWrappedCheck{Check: resolveTestRunner("file", nil)},
			fakeWrappedCheck{Check: resolveTestRunner("kube", nil)},
		}}, true)}

		recorder := callResolveHandler(handler, `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"}}`)

		assert.Equal(t, http.StatusConflict, recorder.Code)
		body := recorder.Body.String()
		assert.Contains(t, body, `"status":"ambiguous_target"`)
		assert.Contains(t, body, `"error":{"code":"ambiguous_target","message":"multiple matching integration checks found"}`)
		assert.NotContains(t, body, "secret-one")
		assert.NotContains(t, body, "secret-two")
	})
}

// TestRemoteQueryResolveHandlerDiagnosticDialect mirrors the match-check endpoint:
// a disabled bridge answers bridge_disabled and malformed requests answer
// invalid_request with fixed messages, never echoed input.
func TestRemoteQueryResolveHandlerDiagnosticDialect(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		handler := &remoteQueryResolveHandler{service: NewRemoteQueryResolveService(resolveTestCollector(), false)}

		recorder := callResolveHandler(handler, `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"}}`)

		assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		assert.Contains(t, recorder.Body.String(), `"status":"bridge_disabled"`)
	})

	t.Run("malformed request", func(t *testing.T) {
		handler := &remoteQueryResolveHandler{service: NewRemoteQueryResolveService(resolveTestCollector(), true)}

		recorder := callResolveHandler(handler, `{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"password":"secret-value"}`)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		body := recorder.Body.String()
		assert.Contains(t, body, `"status":"invalid_request"`)
		assert.Contains(t, body, "request contains unknown field")
		assert.NotContains(t, body, "secret-value")
	})

	t.Run("invalid content type", func(t *testing.T) {
		handler := &remoteQueryResolveHandler{service: NewRemoteQueryResolveService(resolveTestCollector(), true)}
		req := httptest.NewRequest(http.MethodPost, RemoteQueryResolveEndpointPath, strings.NewReader(`{"integration":"postgres"}`))
		req.Header.Set("Content-Type", "text/plain")
		recorder := httptest.NewRecorder()

		handler.handle(recorder, req)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "content-type must be application/json")
	})
}

func callResolveHandler(handler *remoteQueryResolveHandler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, RemoteQueryResolveEndpointPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.handle(recorder, req)
	return recorder
}
