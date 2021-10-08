// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"

	"github.com/stretchr/testify/mock"
)

type mockIterator struct {
	mock.Mock

	els   []eval.Instance
	index int
}

func (m *mockIterator) Next() (eval.Instance, error) {
	_ = m.Called()

	el := m.els[m.index]
	m.index++

	return el, nil
}

func (m *mockIterator) Done() bool {
	_ = m.Called()

	if m.index >= len(m.els) {
		m.index = 0
		return true
	}

	return false
}
