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

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportIssueReturnsNilError(t *testing.T) {
	provides := NewComponent()
	report := &healthplatformpayload.IssueReport{IssueId: "test-issue"}
	err := provides.Comp.ReportIssue("check-1", "mycheck", report)
	require.NoError(t, err)
}

func TestReportIssueWithNilReportReturnsNilError(t *testing.T) {
	provides := NewComponent()
	err := provides.Comp.ReportIssue("check-1", "mycheck", nil)
	require.NoError(t, err)
}

func TestRegisterCheckReturnsNilError(t *testing.T) {
	provides := NewComponent()
	checkFn := func() (*healthplatformpayload.IssueReport, error) {
		return nil, nil
	}
	err := provides.Comp.RegisterCheck("check-1", "mycheck", checkFn, 0)
	require.NoError(t, err)
}

func TestGetAllIssuesReturnsZeroCountAndEmptyMap(t *testing.T) {
	provides := NewComponent()
	count, issues := provides.Comp.GetAllIssues()
	assert.Equal(t, 0, count)
	assert.NotNil(t, issues)
	assert.Empty(t, issues)
}

func TestGetIssueForCheckReturnsNil(t *testing.T) {
	provides := NewComponent()
	issue := provides.Comp.GetIssueForCheck("any-check")
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
		provides.Comp.ClearIssuesForCheck("check-1")
	})
	assert.NotPanics(t, func() {
		provides.Comp.ClearAllIssues()
	})
}
