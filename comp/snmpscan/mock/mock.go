// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the snmpscan component
package mock

import (
	"context"
	"testing"

	snmpscan "github.com/DataDog/datadog-agent/comp/snmpscan/def"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/mock"
)

// SnmpScanMock mocks snmpscan.Component
type SnmpScanMock struct {
	mock.Mock
}

// Mock returns a mock for snmpscan component
func Mock(_ *testing.T) snmpscan.Component {
	return &SnmpScanMock{}
}

// RunSnmpWalk is a mock function
func (m *SnmpScanMock) RunSnmpWalk(snmpConection *gosnmp.GoSNMP, firstOid string) error {
	args := m.Called(snmpConection, firstOid)
	return args.Error(0)
}

// ScanDeviceAndSendData is a mock function
func (m *SnmpScanMock) ScanDeviceAndSendData(ctx context.Context, connParams *snmpparse.SNMPConfig, namespace string, scanParams snmpscan.ScanParams) error {
	args := m.Called(ctx, connParams, namespace, scanParams)
	return args.Error(0)
}
