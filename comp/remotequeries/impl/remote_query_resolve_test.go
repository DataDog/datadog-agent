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

func resolveTestCollector() fakeCollector {
	return fakeCollector{checks: []check.Check{
		fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: LOCALHOST.\nport: 5432\ndbname: postgres\nusername: alice\npassword: secret-value\n"},
		fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5433\ndbname: postgres\npassword: other-secret\n"},
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
// service reuses the shared matcher's zero/one/many semantics with the
// match-before-execute statuses: zero matches answer target_not_found, exactly
// one answers matched with a non-empty fingerprint, and more than one answers
// ambiguous_target. No secret from any instance config surfaces in any result.
func TestRemoteQueryResolveServiceAnswersStructuredOutcomes(t *testing.T) {
	service := NewRemoteQueryResolveService(resolveTestCollector(), true)

	t.Run("zero matches", func(t *testing.T) {
		result := service.Resolve(RemoteQueryResolveRequest{
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

	t.Run("exactly one match", func(t *testing.T) {
		result := service.Resolve(resolveTupleRequest())

		assert.Equal(t, http.StatusOK, result.HTTPStatus)
		assert.Equal(t, statusMatched, result.Status)
		assert.Nil(t, result.Error)
		assert.Regexp(t, fingerprintHexPattern, result.MatchFingerprint)
	})

	// Tiered matcher semantics: the requested dbname is the logical execution
	// database, not an identity requirement on the check's configured dbname —
	// a host+port endpoint candidate with a differing configured dbname is a
	// live match (the dynamic-database targeting contract).
	t.Run("dynamic dbname endpoint candidate is matched", func(t *testing.T) {
		result := service.Resolve(RemoteQueryResolveRequest{
			Integration: "postgres",
			Target:      RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "other"},
		})

		assert.Equal(t, http.StatusOK, result.HTTPStatus)
		assert.Equal(t, statusMatched, result.Status)
		assert.Nil(t, result.Error)
		assert.Regexp(t, fingerprintHexPattern, result.MatchFingerprint)
	})

	t.Run("multiple matches", func(t *testing.T) {
		duplicateService := NewRemoteQueryResolveService(fakeCollector{checks: []check.Check{
			fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\ntags:\n  - rq_database_instance:duplicate\ndatabase_identifier:\n  template: $rq_database_instance\npassword: secret-one\n"},
			fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5433\ndbname: postgres\ntags:\n  - rq_database_instance:duplicate\ndatabase_identifier:\n  template: $rq_database_instance\npassword: secret-two\n"},
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
// selector resolves through the rendered identifier exactly like execute matches.
func TestRemoteQueryResolveServiceDatabaseInstanceTarget(t *testing.T) {
	service := NewRemoteQueryResolveService(fakeCollector{checks: []check.Check{
		fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\ntags:\n  - rq_database_instance:rq-proof-a1-db1\ndatabase_identifier:\n  template: $rq_database_instance\npassword: secret-value\n"},
	}}, true)

	result := service.Resolve(RemoteQueryResolveRequest{
		Integration: "postgres",
		Target:      RemoteQueryExecuteTarget{DatabaseInstance: "rq-proof-a1-db1"},
	})

	assert.Equal(t, http.StatusOK, result.HTTPStatus)
	assert.Equal(t, statusMatched, result.Status)
	assert.Regexp(t, fingerprintHexPattern, result.MatchFingerprint)
}

// TestRemoteQueryResolveServiceFailuresAreResolutionErrors proves every failure
// to complete matching answers resolution_error — never a silent target miss: a
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
		handler := &remoteQueryResolveHandler{service: NewRemoteQueryResolveService(resolveTestCollector(), true)}

		recorder := callResolveHandler(handler, `{"integration":"postgres","target":{"host":"nowhere","port":5432,"dbname":"other"}}`)

		assert.Equal(t, http.StatusNotFound, recorder.Code)
		body := recorder.Body.String()
		assert.Contains(t, body, `"status":"target_not_found"`)
		assert.Contains(t, body, `"error":{"code":"target_not_found","message":"no matching integration check found"}`)
		assert.NotContains(t, body, `"matchFingerprint"`)
	})

	t.Run("ambiguous", func(t *testing.T) {
		handler := &remoteQueryResolveHandler{service: NewRemoteQueryResolveService(fakeCollector{checks: []check.Check{
			fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-one\n"},
			fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-two\n"},
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
