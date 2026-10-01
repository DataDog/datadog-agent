// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestIdentifierConfigConfigured(t *testing.T) {
	tests := []struct {
		name string
		in   IdentifierConfig
		want bool
	}{
		{"empty", IdentifierConfig{}, false},
		{"label", IdentifierConfig{Key: "a/b", Type: IdentifierTypeLabel}, true},
		{"annotation", IdentifierConfig{Key: "a/b", Type: IdentifierTypeAnnotation}, true},
		{"env", IdentifierConfig{Key: "JOB_ID", Type: IdentifierTypeEnv}, true},
		{"missing key", IdentifierConfig{Type: IdentifierTypeLabel}, false},
		{"unknown type", IdentifierConfig{Key: "a/b", Type: "other"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.in.Configured())
		})
	}
}

func TestNewJobsConfig(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("gpu.jobs.run.key", "_RAY_SUBMISSION_ID")
	cfg.SetInTest("gpu.jobs.run.type", "env")
	cfg.SetInTest("gpu.jobs.group.key", "example/job-group-annotation")
	cfg.SetInTest("gpu.jobs.group.type", "Annotation")

	got := New()

	assert.Equal(t, NewJobsConfig(), got.JobsConfig)
	assert.Equal(t, IdentifierConfig{Key: "_RAY_SUBMISSION_ID", Type: IdentifierTypeEnv}, got.JobsConfig.Run)
	assert.Equal(t, IdentifierConfig{Key: "example/job-group-annotation", Type: IdentifierTypeAnnotation}, got.JobsConfig.Group)
}

func TestNewJobsConfigDefaults(t *testing.T) {
	configmock.New(t)

	got := New()

	assert.Equal(t, IdentifierConfig{Type: IdentifierTypeLabel}, got.JobsConfig.Run)
	assert.Equal(t, IdentifierConfig{Type: IdentifierTypeLabel}, got.JobsConfig.Group)
	assert.False(t, got.JobsConfig.Run.Configured())
	assert.False(t, got.JobsConfig.Group.Configured())
}
