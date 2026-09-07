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
}
