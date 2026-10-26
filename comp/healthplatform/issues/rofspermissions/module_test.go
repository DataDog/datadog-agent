// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build docker

package rofspermissions

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
)

func TestInstanceIssueIDScopedByRoleAndResource(t *testing.T) {
	for _, tc := range []struct {
		name       string
		clcRunner  bool
		expectedID string
	}{
		{name: "node agent", clcRunner: false, expectedID: "rofs-permissions:agent:node-1"},
		{name: "cluster checks runner", clcRunner: true, expectedID: "rofs-permissions:clc-runner:node-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hostname, _ := hostnamemock.NewMock("node-1")
			module := &rofsPermissionsModule{
				conf:     config.NewMockWithOverrides(t, map[string]interface{}{"clc_runner_enabled": tc.clcRunner}),
				hostname: hostname,
			}

			assert.Equal(t, tc.expectedID, module.instanceIssueID())
		})
	}
}
