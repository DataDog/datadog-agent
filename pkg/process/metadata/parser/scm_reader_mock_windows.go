// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && windows

package parser

import (
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

var _ mockableSCM = (*mockSCM)(nil)

type mockSCM struct {
	mock.Mock
}

func newSCMReaderWithMock(t *testing.T) (*scmReader, *mockSCM) {
	m := &mockSCM{}
	m.Test(t)
	t.Cleanup(func() { m.AssertExpectations(t) })
	return &scmReader{
		scmMonitor: m,
	}, m
}

func (m *mockSCM) GetServiceInfo(pid uint64) (*winutil.ServiceInfo, error) {
	args := m.Called(pid)

	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*winutil.ServiceInfo), args.Error(1)
}
