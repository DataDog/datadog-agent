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
		pids           []uint32
		paths          []string
		expectedResult bool
	}{
		{[]uint32{pid + 1}, []string{"/usr/bin/date"}, true},
		{[]uint32{pid + 1}, []string{"/usr/bin/dd"}, false},
		{[]uint32{pid + 1}, []string{"/usr/sbin/sudo"}, false},
		{[]uint32{pid}, []string{"/usr/bin/date"}, false},
		{[]uint32{1}, []string{"/usr/bin/date"}, false},
		{[]uint32{pid + 1}, []string{"/opt/datadog-agent/bin/agent/agent"}, false},
		{[]uint32{pid + 1}, []string{"/opt/datadog-packages/datadog-agent/v1.0.0/bin/agent/agent"}, false},
	}

	for _, test := range tests {
		isKilledAllowed, _ := p.isKillAllowed(test.pids, test.paths)
		assert.Equal(t, test.expectedResult, isKilledAllowed)
	}
}
