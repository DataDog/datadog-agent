// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestAdjustSecurityCgroupDumpTimeout(t *testing.T) {
	tests := []struct {
		name              string
		cgroupDumpTimeout *int
		dumpDuration      *string
		expected          time.Duration
	}{
		{name: "neither set", expected: 15 * time.Minute},
		{name: "only old key", cgroupDumpTimeout: new(20), expected: 20 * time.Minute},
		{name: "only new key", dumpDuration: new("30m"), expected: 30 * time.Minute},
		{name: "new key wins over old key", cgroupDumpTimeout: new(20), dumpDuration: new("30m"), expected: 30 * time.Minute},
	}

	for _, te := range tests {
		t.Run(te.name, func(t *testing.T) {
			cfg := mock.NewSystemProbe(t)
			if te.cgroupDumpTimeout != nil {
				cfg.Set(secNS("activity_dump.cgroup_dump_timeout"), *te.cgroupDumpTimeout, model.SourceFile)
			}
			if te.dumpDuration != nil {
				cfg.Set(secNS("activity_dump.dump_duration"), *te.dumpDuration, model.SourceFile)
			}
			adjustSecurity(cfg)

			assert.Equal(t, te.expected, cfg.GetDuration(secNS("activity_dump.dump_duration")))
		})
	}
}

func TestAdjustSecurityDisablesSecurityProfileWithoutCWS(t *testing.T) {
	tests := []struct {
		name       string
		cwsEnabled bool
	}{
		{name: "CWS enabled", cwsEnabled: true},
		{name: "CWS disabled", cwsEnabled: false},
	}

	for _, te := range tests {
		t.Run(te.name, func(t *testing.T) {
			cfg := mock.NewSystemProbe(t)
			cfg.Set(secNS("enabled"), te.cwsEnabled, model.SourceFile)
			cfg.Set(secNS("security_profile.v2.enabled"), true, model.SourceFile)
			adjustSecurity(cfg)

			assert.Equal(t, te.cwsEnabled, cfg.GetBool(secNS("security_profile.v2.enabled")))
		})
	}
}
