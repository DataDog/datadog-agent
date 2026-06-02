// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package healthplatformnoopimpl

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
)

func TestReportIssueReturnsNilError(t *testing.T) {
	provides := NewComponent()
	err := provides.Comp.ReportIssue(&healthplatformpayload.Issue{
		Id:        "check-1:instance-1",
		IssueName: "test-issue",
		Source:    "mycheck",
	})
	require.NoError(t, err)
}

func TestGetAllIssuesReturnsZeroCountAndEmptyMap(t *testing.T) {
	provides := NewComponent()
	count, issues := provides.Comp.GetAllIssues()
	assert.Equal(t, 0, count)
	assert.NotNil(t, issues)
	assert.Empty(t, issues)
}

func TestGetIssueReturnsNil(t *testing.T) {
	provides := NewComponent()
	issue := provides.Comp.GetIssue("any-check")
	assert.Nil(t, issue)
}

func TestAPIGetIssuesEndpoint(t *testing.T) {
	provides := NewComponent()
	provider := provides.APIGetIssues.Provider
	require.NotNil(t, provider)
	assert.Equal(t, "/health-platform/issues", provider.Route())

	handler := provider.HandlerFunc()
	require.NotNil(t, handler)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health-platform/issues", nil)
	handler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"count":0,"issues":{}}`, string(body))
}

func TestFlareProviderCallbackReturnsNil(t *testing.T) {
	provides := NewComponent()
	require.NotNil(t, provides.FlareProvider.FlareFiller)
	require.NotNil(t, provides.FlareProvider.FlareFiller.Callback)
	err := provides.FlareProvider.FlareFiller.Callback(nil, nil)
	assert.NoError(t, err)
}

func TestClearMethodsDoNotPanic(t *testing.T) {
	provides := NewComponent()
	assert.NotPanics(t, func() {
		provides.Comp.ResolveIssue("check-1")
	})
	assert.NotPanics(t, func() {
		provides.Comp.ResolveAllIssues()
	})
}
