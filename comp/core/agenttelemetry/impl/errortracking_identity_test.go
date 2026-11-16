// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenttelemetryimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
)

func TestBuildIdentityTags(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{"release", "7.85.0", ",agent.version:7.85.0,agent.rc:false"},
		{"release candidate", "7.85.0-rc.2", ",agent.version:7.85.0-rc.2,agent.rc:true"},
		{"development build", "7.86.0-devel+git.12.abcdef0", ",agent.version:7.86.0-devel,agent.rc:false"},
		// Build metadata is dropped so the tag matches the telemetry header.
		{"release candidate with metadata", "7.85.0-rc.8+git.3.1234567", ",agent.version:7.85.0-rc.8,agent.rc:true"},
		// A segment that merely starts with "rc" is not a release candidate.
		{"rc-like segment", "7.85.0-rcfoo.1", ",agent.version:7.85.0-rcfoo.1,agent.rc:false"},
		{"unparsable version emits nothing", "not-a-version", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, buildIdentityTags(tt.version))
		})
	}
}

func TestEnrichErrorLog_CarriesIdentityTags(t *testing.T) {
	out := enrichErrorLog(errortracking.ErrorLog{})
	assert.Contains(t, out.Tags, ",agent.version:")
	assert.Contains(t, out.Tags, ",agent.rc:")
}
