// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pinger

type mockPinger struct {
	res *Result
	err error
}

func NewMockPinger(res *Result, err error) *mockPinger {
	return &mockPinger{
		res: res,
		err: err,
	}
}

func (m *mockPinger) Ping(host string) (*Result, error) {
	return m.res, m.err
}
