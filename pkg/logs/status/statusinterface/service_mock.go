// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package statusinterface

type mockStatusService struct{}

// AddGlobalWarning keeps track of a warning message to display on the status.
func (ms *mockStatusService) AddGlobalWarning(string, string) {
}

// RemoveGlobalWarning loses track of a warning message
// that does not need to be displayed on the status anymore.
func (ms *mockStatusService) RemoveGlobalWarning(string) {
}

// NewStatusMock returns a mock instance of statusinterface to be used in tests
func NewStatusMock() Component {
	return &mockStatusService{}
}
