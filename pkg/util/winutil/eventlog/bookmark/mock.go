// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows && test

package evtbookmark

import (
	"github.com/stretchr/testify/mock"
)

// MockSaver is a testify mock implementation of Saver
type MockSaver struct {
	mock.Mock
}

// Save saves the bookmark XML to the saver
func (m *MockSaver) Save(bookmarkXML string) error {
	args := m.Called(bookmarkXML)
	return args.Error(0)
}

// Load loads the bookmark XML from the saver
func (m *MockSaver) Load() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}
