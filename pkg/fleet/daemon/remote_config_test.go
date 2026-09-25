// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

var (
	testAgentCatalog = catalog{
		Packages: []Package{
			{
				Name:    "datadog-agent",
				Version: "7.31.0",
				URL:     "https://example.com/datadog-agent-7.31.0.tar",
			},
		},
	}
	testAgentCatalogBadOCIWithTag = catalog{
		Packages: []Package{
			{
				Name:    "datadog-agent",
				Version: "7.31.0",
				URL:     "oci://example.com/datadog-agent:7.31.0",
			},
		},
	}
	testTracerCatalog = catalog{
		Packages: []Package{
			{
				Name:    "dd-trace-py",
				Version: "1.31.0",
				URL:     "oci://example.com/dd-trace-py@sha256:2a5ca68f1f0a088cdf1cd1efa086ffe0ca80f8339c7fa12a7f41bbe9d1527cb6",
			},
		},
	}
	testCatalog = catalog{
		Packages: append(testAgentCatalog.Packages, testTracerCatalog.Packages...),
	}
	testAgentCatalogJSON, _              = json.Marshal(testAgentCatalog)
	testAgentCatalogBadOCIWithTagJSON, _ = json.Marshal(testAgentCatalogBadOCIWithTag)
	testTracerCatalogJSON, _             = json.Marshal(testTracerCatalog)
)

var (
	testRemoteAPIRequest = remoteAPIRequest{
		ID: "test",
		ExpectedState: expectedState{
			Stable: "7.31.0",
		},
		Method: "start_experiment",
		Params: json.RawMessage(`{"version":"7.32.0"}`),
	}
	testRemoteAPIRequestJSON, _ = json.Marshal(testRemoteAPIRequest)
)

func alwaysCatalogReady() bool { return true }

type callbackMock struct {
	mock.Mock
}

func (c *callbackMock) handleCatalogUpdate(catalog catalog) error {
	args := c.Called(catalog)
	return args.Error(0)
}

func (c *callbackMock) handleRemoteAPIRequest(request remoteAPIRequest) error {
	args := c.Called(request)
	return args.Error(0)
}

func (c *callbackMock) applyStateCallback(id string, status state.ApplyStatus) {
	c.Called(id, status)
}

func (c *callbackMock) firstCatalogApplied() {
	c.Called()
}

func TestCatalogUpdate(t *testing.T) {
	callback := &callbackMock{}
	handler := handleUpdaterCatalogDDUpdate(callback.handleCatalogUpdate, callback.firstCatalogApplied)
	callback.On("firstCatalogApplied").Return()
	callback.On("handleCatalogUpdate", mock.MatchedBy(func(catalog catalog) bool {
		return assert.ElementsMatch(t, testCatalog.Packages, catalog.Packages)
	})).Return(nil)
	callback.
		On("applyStateCallback", "agent", state.ApplyStatus{State: state.ApplyStateAcknowledged}).
		On("applyStateCallback", "tracer", state.ApplyStatus{State: state.ApplyStateAcknowledged}).
		Return()

	handler(map[string]state.RawConfig{
		"agent":  {Config: testAgentCatalogJSON},
		"tracer": {Config: testTracerCatalogJSON},
	}, callback.applyStateCallback)

	callback.AssertExpectations(t)
}

func TestCatalogUpdateBadConfig(t *testing.T) {
	callback := &callbackMock{}
	handler := handleUpdaterCatalogDDUpdate(callback.handleCatalogUpdate, callback.firstCatalogApplied)
	callback.On("applyStateCallback", "test", mock.MatchedBy(func(s state.ApplyStatus) bool {
		return s.State == state.ApplyStateError
	})).Return()

	handler(map[string]state.RawConfig{
		"test": {Config: []byte("bad json")},
	}, callback.applyStateCallback)

	callback.AssertExpectations(t)
}

func TestCatalogUpdateError(t *testing.T) {
	callback := &callbackMock{}
	handler := handleUpdaterCatalogDDUpdate(callback.handleCatalogUpdate, callback.firstCatalogApplied)
	err := errors.New("test error")
	callback.On("handleCatalogUpdate", mock.Anything).Return(err)
	callback.
		On("applyStateCallback", "agent", state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()}).
		On("applyStateCallback", "tracer", state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()}).
		Return()

	handler(map[string]state.RawConfig{
		"agent":  {Config: testAgentCatalogJSON},
		"tracer": {Config: testTracerCatalogJSON},
	}, callback.applyStateCallback)

	callback.AssertExpectations(t)
}

func TestCatalogUpdateBadPackageWithOCITag(t *testing.T) {
	callback := &callbackMock{}
	handler := handleUpdaterCatalogDDUpdate(callback.handleCatalogUpdate, callback.firstCatalogApplied)
	callback.On("applyStateCallback", "agent", mock.MatchedBy(func(s state.ApplyStatus) bool {
		return s.State == state.ApplyStateError
	})).Return()

	handler(map[string]state.RawConfig{
		"agent": {Config: testAgentCatalogBadOCIWithTagJSON},
	}, callback.applyStateCallback)

	callback.AssertExpectations(t)
}

func TestCatalogUpdateFirstCatalogAppliedCallback(t *testing.T) {
	callback := &callbackMock{}
	handler := handleUpdaterCatalogDDUpdate(callback.handleCatalogUpdate, callback.firstCatalogApplied)
	callback.On("firstCatalogApplied").Return().Once()
	callback.On("handleCatalogUpdate", mock.Anything).Return(nil).Times(2)
	callback.On("applyStateCallback", "agent", state.ApplyStatus{State: state.ApplyStateAcknowledged}).Return().Times(2)

	handler(map[string]state.RawConfig{
		"agent": {Config: testAgentCatalogJSON},
	}, callback.applyStateCallback)
	handler(map[string]state.RawConfig{
		"agent": {Config: testAgentCatalogJSON},
	}, callback.applyStateCallback)

	callback.AssertExpectations(t)
}

func TestRemoteAPIRequest(t *testing.T) {
	callback := &callbackMock{}
	handler := handleUpdaterTaskUpdate(callback.handleRemoteAPIRequest, alwaysCatalogReady)
	callback.On("handleRemoteAPIRequest", testRemoteAPIRequest).Return(nil)
	callback.On("applyStateCallback", "test", state.ApplyStatus{State: state.ApplyStateAcknowledged}).Return()

	handler(map[string]state.RawConfig{
		"test": {Config: testRemoteAPIRequestJSON},
	}, callback.applyStateCallback)

	callback.AssertExpectations(t)
}

func TestRemoteAPIRequestBadConfig(t *testing.T) {
	callback := &callbackMock{}
	handler := handleUpdaterTaskUpdate(callback.handleRemoteAPIRequest, alwaysCatalogReady)
	callback.On("applyStateCallback", "test", mock.MatchedBy(func(s state.ApplyStatus) bool {
		return s.State == state.ApplyStateError
	})).Return()

	handler(map[string]state.RawConfig{
		"test": {Config: []byte("bad json")},
	}, callback.applyStateCallback)

	callback.AssertExpectations(t)
}

func TestRemoteAPIRequestError(t *testing.T) {
	callback := &callbackMock{}
	handler := handleUpdaterTaskUpdate(callback.handleRemoteAPIRequest, alwaysCatalogReady)
	err := errors.New("test error")
	callback.On("handleRemoteAPIRequest", mock.Anything).Return(err)
	callback.On("applyStateCallback", "test", state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()}).Return()

	handler(map[string]state.RawConfig{
		"test": {Config: testRemoteAPIRequestJSON},
	}, callback.applyStateCallback)

	callback.AssertExpectations(t)
}

func TestRemoteAPIRequestIgnoresAlreadyExecutedRequests(t *testing.T) {
	callback := &callbackMock{}
	handler := handleUpdaterTaskUpdate(callback.handleRemoteAPIRequest, alwaysCatalogReady)
	callback.On("handleRemoteAPIRequest", testRemoteAPIRequest).Return(nil)
	callback.On("applyStateCallback", "test1", state.ApplyStatus{State: state.ApplyStateAcknowledged}).Times(1).Return()

	handler(map[string]state.RawConfig{
		"test1": {Config: testRemoteAPIRequestJSON},
	}, callback.applyStateCallback)

	handler(map[string]state.RawConfig{
		"test1": {Config: testRemoteAPIRequestJSON},
	}, callback.applyStateCallback)

	callback.AssertExpectations(t)
}

// TestRemoteAPIRequestWaitsForCatalog pins the property the UPDATER_TASK/UPDATER_CATALOG_DD
// decoupling relies on: subscribing to tasks no longer waits for a catalog, but executing one
// still does. A task delivered before any catalog has been applied must be left unacknowledged,
// not executed or errored, so remote-config redelivers it once a catalog exists.
func TestRemoteAPIRequestWaitsForCatalog(t *testing.T) {
	callback := &callbackMock{}
	handler := handleUpdaterTaskUpdate(callback.handleRemoteAPIRequest, func() bool { return false })

	handler(map[string]state.RawConfig{
		"test": {Config: testRemoteAPIRequestJSON},
	}, callback.applyStateCallback)

	callback.AssertNotCalled(t, "handleRemoteAPIRequest", mock.Anything)
	callback.AssertNotCalled(t, "applyStateCallback", mock.Anything, mock.Anything)
}
