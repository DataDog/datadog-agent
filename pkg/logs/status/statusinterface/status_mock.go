// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package statusinterface

import "io"

type mockStatusProvider struct{}

func (mp *mockStatusProvider) Name() string {
	return "Logs Agent"
}

func (mp *mockStatusProvider) Section() string {
	return "Logs Agent"
}

func (mp *mockStatusProvider) JSON(_ bool, _ map[string]interface{}) error {
	return nil
}

func (mp *mockStatusProvider) Text(_ bool, _ io.Writer) error {
	return nil
}

func (mp *mockStatusProvider) HTML(_ bool, _ io.Writer) error {
	return nil
}

// AddGlobalWarning keeps track of a warning message to display on the status.
func (mp *mockStatusProvider) AddGlobalWarning(string, string) {
}

// RemoveGlobalWarning loses track of a warning message
// that does not need to be displayed on the status anymore.
func (mp *mockStatusProvider) RemoveGlobalWarning(string) {
}

// NewStatusProviderMock returns a mock instance of statusinterface to be used in tests
func NewStatusProviderMock() Status {
	return &mockStatusProvider{}
}
