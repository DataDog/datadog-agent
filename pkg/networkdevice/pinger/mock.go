// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pinger

// MockPinger is a pinger used for testing
type MockPinger struct {
	res *Result
	err error
}

// NewMockPinger returns a pinger that
func NewMockPinger(res *Result, err error) *MockPinger {
	return &MockPinger{
		res: res,
		err: err,
	}
}

// Ping ignores the passed in host and returns the result
// and error set in the constructor
func (m *MockPinger) Ping(_ string) (*Result, error) {
	return m.res, m.err
}
