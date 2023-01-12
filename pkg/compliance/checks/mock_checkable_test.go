// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"

	"github.com/stretchr/testify/mock"
)

type mockCheckable struct {
	mock.Mock
}

func (m *mockCheckable) Check(env env.Env) []*compliance.Report {
	args := m.Called(env)
	reports := args.Get(0).([]*compliance.Report)
	return reports
}
