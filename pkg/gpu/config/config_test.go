// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

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
		{"missing key", IdentifierConfig{Type: IdentifierTypeLabel}, false},
		{"env", IdentifierConfig{Key: "JOB_ID", Type: IdentifierTypeEnv}, true},
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
	cfg.SetInTest("gpu.jobs.run.key", "example/job-id-label")
	cfg.SetInTest("gpu.jobs.run.type", "label")
	cfg.SetInTest("gpu.jobs.group.key", "example/job-group-annotation")
	cfg.SetInTest("gpu.jobs.group.type", "Annotation")

	got := New()

	assert.Equal(t, got.JobsConfig, NewJobsConfig(cfg))
	assert.Equal(t, IdentifierConfig{Key: "example/job-id-label", Type: IdentifierTypeLabel}, got.JobsConfig.Run)
	assert.Equal(t, IdentifierConfig{Key: "example/job-group-annotation", Type: IdentifierTypeAnnotation}, got.JobsConfig.Group)
}

func TestNewJobsConfigDefaults(t *testing.T) {
	configmock.New(t)

	got := New()

	assert.False(t, got.JobsConfig.Run.Configured())
	assert.False(t, got.JobsConfig.Group.Configured())
}

func TestJobsConfigEnvKeys(t *testing.T) {
	label := IdentifierConfig{Key: "example/job-id-label", Type: IdentifierTypeLabel}
	envRun := IdentifierConfig{Key: "JOB_ID", Type: IdentifierTypeEnv}
	envGroup := IdentifierConfig{Key: "JOB_GROUP", Type: IdentifierTypeEnv}

	assert.Empty(t, JobsConfig{}.EnvKeys())
	assert.Empty(t, JobsConfig{Run: label}.EnvKeys())
	assert.Equal(t, []string{"JOB_ID"}, JobsConfig{Run: envRun, Group: label}.EnvKeys())
	assert.Equal(t, []string{"JOB_ID", "JOB_GROUP"}, JobsConfig{Run: envRun, Group: envGroup}.EnvKeys())
	assert.Equal(t, []string{"JOB_ID"}, JobsConfig{Run: envRun, Group: envRun}.EnvKeys(), "keys are not duplicated")
	assert.Empty(t, JobsConfig{Run: IdentifierConfig{Type: IdentifierTypeEnv}}.EnvKeys(), "an env identifier needs a key")
}
