// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package inventorysoftware

import (
	"github.com/stretchr/testify/mock"
	types "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

// mockSysProbeClient implements SysProbeClient for testing.
type mockSysProbeClient struct {
	mock.Mock
}

func (m *mockSysProbeClient) GetCheck(module types.ModuleName) (SoftwareInventoryMap, error) {
	args := m.Called(module)
	return args.Get(0).(SoftwareInventoryMap), args.Error(1)
}

// mockFlareBuilder implements a minimal FlareBuilder for testing
type mockFlareBuilder struct {
    addedFiles map[string][]byte
}

func (m *mockFlareBuilder) AddFile(filepath string, content []byte) error {
    if m.addedFiles == nil {
        m.addedFiles = make(map[string][]byte)
    }
    m.addedFiles[filepath] = content
    return nil
}

func (m *mockFlareBuilder) Logf(format string, args ...interface{}) error {
    return nil
}