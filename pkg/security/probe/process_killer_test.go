// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

func TestProcessKillerExclusion(t *testing.T) {
	p, err := NewProcessKiller(
		&config.Config{
			RuntimeSecurity: &config.RuntimeSecurityConfig{
				EnforcementBinaryExcluded: []string{
					"/usr/bin/dd",
					"/usr/sbin/*",
					"/var/lib/*/runc",
				},
			},
		},
	)
	assert.Nil(t, err)

	pid := utils.Getpid()
	tests := []struct {
		pids           uint32
		paths          string
		expectedResult bool
	}{
		{pid + 1, "/usr/bin/date", true},
		{pid + 1, "/usr/bin/dd", false},
		{pid + 1, "/usr/sbin/sudo", false},
		{pid, "/usr/bin/date", false},
		{1, "/usr/bin/date", false},
		{pid + 1, "/opt/datadog-agent/bin/agent/agent", false},
		{pid + 1, "/opt/datadog-packages/datadog-agent/v1.0.0/bin/agent/agent", false},
	}

	for _, test := range tests {
		isKilledAllowed, _ := p.isKillAllowed([]processContext{{pid: int(test.pids), path: test.paths}})
		assert.Equal(t, test.expectedResult, isKilledAllowed)
	}
}
