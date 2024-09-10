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
	assert.True(t, p.isKillAllowed([]uint32{utils.Getpid() + 1}, []string{"/usr/bin/date"}))
	assert.False(t, p.isKillAllowed([]uint32{utils.Getpid() + 1}, []string{"/usr/bin/dd"}))
	assert.False(t, p.isKillAllowed([]uint32{utils.Getpid() + 1}, []string{"/usr/sbin/sudo"}))
	assert.False(t, p.isKillAllowed([]uint32{utils.Getpid()}, []string{"/usr/bin/date"}))
	assert.False(t, p.isKillAllowed([]uint32{1}, []string{"/usr/bin/date"}))
	assert.False(t, p.isKillAllowed([]uint32{utils.Getpid() + 1}, []string{"/opt/datadog-agent/bin/agent/agent"}))
	assert.False(t, p.isKillAllowed([]uint32{utils.Getpid() + 1}, []string{"/opt/datadog-packages/datadog-agent/v1.0.0/bin/agent/agent"}))
}
