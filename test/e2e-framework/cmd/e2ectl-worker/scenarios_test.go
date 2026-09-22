// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/workerclient"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/fixtures"
)

func TestEC2SharedSchemaValidation(t *testing.T) {
	for _, params := range []string{"", "os: unknown", "os: ubuntu-22.04\narch: unknown", "os: ubuntu-22.04\nextra: x", "os: ubuntu-22.04\nfakeintake: true"} {
		if _, err := buildEC2Host(params, fixtures.Config{FakeIntake: true}); err == nil {
			t.Fatalf("executor should reject invalid parameters: %q", params)
		}
	}
	for _, enabled := range []bool{false, true} {
		// Construction only: no Pulumi execution, credentials or cloud calls.
		exec, err := buildEC2Host("os: ubuntu-22.04\narch: amd64", fixtures.Config{FakeIntake: enabled})
		if err != nil {
			t.Fatal(err)
		}
		if exec.Provision == nil || exec.Destroy == nil {
			t.Fatal("missing scenario lifecycle")
		}
	}
}

func TestExecutorRejectsOldProtocolBeforeProvisioning(t *testing.T) {
	if err := runJob(workerclient.Job{Base: workerclient.BaseEC2Host, Action: workerclient.ActionProvision}); err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("old requests must fail before using defaults or creating infrastructure: %v", err)
	}
}

func TestOSMappingDoesNotSilentlyFallBack(t *testing.T) {
	if _, err := osDescriptor("unknown"); err == nil {
		t.Fatal("unknown OS must not silently become Ubuntu")
	}
	if _, err := dockerOSDescriptor("unknown"); err == nil {
		t.Fatal("unknown Docker host OS must not silently become the default AMI")
	}
}

// Construction only: no Pulumi execution, credentials or cloud calls. The
// shared schemas' semantic rules (EKS topology, Docker AMI availability) run
// on both sides of the process boundary, so rejecting them here proves the
// executor rejects them too.
func TestEKSSharedSchemaValidation(t *testing.T) {
	for _, params := range []string{
		"windows: true\nlinux: false",
		"linux: false",
		"version: 1.31\n",
		"linux: true\nextra: x",
	} {
		if _, err := buildEKS(params, fixtures.Config{FakeIntake: true}); err == nil {
			t.Fatalf("executor should reject invalid EKS parameters: %q", params)
		}
	}
	for _, params := range []string{
		// Normalized payload shape the CLI forwards: every default explicit.
		"linux: true\nwindows: false\nversion: '1.34'",
		"linux: true\nwindows: true\nversion: '1.32'",
	} {
		for _, fakeintake := range []bool{false, true} {
			exec, err := buildEKS(params, fixtures.Config{FakeIntake: fakeintake})
			if err != nil {
				t.Fatal(params, err)
			}
			if exec.Provision == nil || exec.Destroy == nil {
				t.Fatal("missing scenario lifecycle")
			}
		}
	}
}

func TestDockerHostSharedSchemaValidation(t *testing.T) {
	for _, params := range []string{
		"os: unknown",
		"os: ubuntu-22.04-e2e\narch: unknown",
		"os: ubuntu-24.04-e2e\narch: arm64",
		"os: ubuntu-22.04-e2e\nfakeintake: true",
		"os: ubuntu-22.04-e2e\nextra: x",
	} {
		if _, err := buildDockerHost(params, fixtures.Config{FakeIntake: true}); err == nil {
			t.Fatalf("executor should reject invalid Docker host parameters: %q", params)
		}
	}
	for _, enabled := range []bool{false, true} {
		exec, err := buildDockerHost("os: ubuntu-22.04-e2e\narch: amd64", fixtures.Config{FakeIntake: enabled})
		if err != nil {
			t.Fatal(err)
		}
		if exec.Provision == nil || exec.Destroy == nil {
			t.Fatal("missing scenario lifecycle")
		}
	}
}
