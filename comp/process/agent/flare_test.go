// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	checkMocks "github.com/DataDog/datadog-agent/pkg/process/checks/mocks"
)

func TestFillFlare(t *testing.T) {
	f := helpers.NewFlareBuilderMock(t, false)

	check := &checkMocks.Check{}
	check.On("Name").Return("process")
	check.On("Realtime").Return(false)

	fc := NewFlareHelper([]checks.Check{check})

	fc.FillFlare(f.Fb)
	f.AssertFileExists("process_check_output.json")
}
