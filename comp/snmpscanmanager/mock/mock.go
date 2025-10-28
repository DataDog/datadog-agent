// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the snmpscanmanager component
package mock

import (
	"testing"

	snmpscanmanager "github.com/DataDog/datadog-agent/comp/snmpscanmanager/def"
)

type snmpScanManagerMock struct {
}

// Mock returns a mock for snmpscanmanager component.
func Mock(_ *testing.T) snmpscanmanager.Component {
	scanManager := &snmpScanManagerMock{}
	return scanManager
}

func (m *snmpScanManagerMock) RequestScan(_ snmpscanmanager.ScanRequest) {
}
