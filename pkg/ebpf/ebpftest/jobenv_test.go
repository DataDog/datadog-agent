// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package ebpftest

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJobEnv(t *testing.T) {
	if os.Getenv("GITLAB_CI") != "true" {
		t.Skip()
	}
	// this is testing logic in test/new-e2e/system-probe/test-runner/main.go
	// to ensure vars from job_env.txt are added to the KMT env vars
	assert.NotEmpty(t, os.Getenv("CI_JOB_URL"))
	assert.NotEmpty(t, os.Getenv("CI_JOB_ID"))
	assert.NotEmpty(t, os.Getenv("CI_JOB_NAME"))
	assert.NotEmpty(t, os.Getenv("CI_JOB_STAGE"))
}
