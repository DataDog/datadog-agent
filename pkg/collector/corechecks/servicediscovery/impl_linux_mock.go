// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Code generated by MockGen. DO NOT EDIT.
// Source: impl_linux.go

// Package servicediscovery is a generated GoMock package.
package servicediscovery

import (
	reflect "reflect"

	model "github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	gomock "github.com/golang/mock/gomock"
)

// MocksystemProbeClient is a mock of systemProbeClient interface.
type MocksystemProbeClient struct {
	ctrl     *gomock.Controller
	recorder *MocksystemProbeClientMockRecorder
}

// MocksystemProbeClientMockRecorder is the mock recorder for MocksystemProbeClient.
type MocksystemProbeClientMockRecorder struct {
	mock *MocksystemProbeClient
}

// NewMocksystemProbeClient creates a new mock instance.
func NewMocksystemProbeClient(ctrl *gomock.Controller) *MocksystemProbeClient {
	mock := &MocksystemProbeClient{ctrl: ctrl}
	mock.recorder = &MocksystemProbeClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MocksystemProbeClient) EXPECT() *MocksystemProbeClientMockRecorder {
	return m.recorder
}

// GetDiscoveryListeners mocks base method.
func (m *MocksystemProbeClient) GetDiscoveryServices() (*model.ServicesResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetDiscoveryServices")
	ret0, _ := ret[0].(*model.ServicesResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetDiscoveryServices indicates an expected call of GetDiscoveryServices.
func (mr *MocksystemProbeClientMockRecorder) GetDiscoveryServices() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetDiscoveryServices", reflect.TypeOf((*MocksystemProbeClient)(nil).GetDiscoveryServices))
}