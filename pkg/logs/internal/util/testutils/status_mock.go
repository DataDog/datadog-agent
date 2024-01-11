// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils

import "github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface"

type mockStatusService struct{}

// AddGlobalWarning keeps track of a warning message to display on the status.
func (ms *mockStatusService) AddGlobalWarning(key string, warning string) {
}

// RemoveGlobalWarning loses track of a warning message
// that does not need to be displayed on the status anymore.
func (ms *mockStatusService) RemoveGlobalWarning(key string) {
}

// NewStatusImpl fetches the status and returns a service wrapping it
func NewStatusMock() statusinterface.StatusInterface {
	return &mockStatusService{}
}
