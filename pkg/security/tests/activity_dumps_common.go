// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests
// +build functionaltests

package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
)

// v see test/kitchen/test/integration/security-agent-test/rspec/security-agent-test_spec.rb
const dedicatedADNodeForTestsEnv = "DEDICATED_ACTIVITY_DUMP_NODE"

const testActivityDumpRateLimiter = 200
const testActivityDumpTracedCgroupsCount = 3
const testActivityDumpCgroupDumpTimeout = 11 // probe.MinDumpTimeout(10) + 5

func validateActivityDumpOutputs(t *testing.T, test *testModule, expectedFormats []string, outputFiles []string,
	activityDumpValidator func(ad *dump.ActivityDump) bool,
	securityProfileValidator func(sp *profile.SecurityProfile) bool) {
	perExtOK := make(map[string]bool)
	for _, format := range expectedFormats {
		ext := fmt.Sprintf(".%s", format)
		perExtOK[ext] = false
	}

	for _, f := range outputFiles {
		ext := filepath.Ext(f)
		if perExtOK[ext] {
			t.Fatalf("Got more than one `%s` file: %v", ext, outputFiles)
		}

		switch ext {
		case ".json":
			content, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if !validateActivityDumpProtoSchema(t, string(content)) {
				t.Error(string(content))
			}
			perExtOK[ext] = true

		case ".protobuf":
			ad, err := test.DecodeActivityDump(f)
			if err != nil {
				t.Fatal(err)
			}

			if activityDumpValidator != nil {
				found := activityDumpValidator(ad)
				if !found {
					t.Error("Invalid activity dump")
				}
				perExtOK[ext] = found
			} else {
				t.Error("No activity dump validator provided")
				perExtOK[ext] = false
			}

		case ".profile":
			profile, err := DecodeSecurityProfile(f)
			if err != nil {
				t.Fatal(err)
			} else if profile == nil {
				t.Error("no profile found")
			}
			if securityProfileValidator != nil {
				found := securityProfileValidator(profile)
				if !found {
					t.Error("Invalid security profile")
				}
				perExtOK[ext] = found
			} else {
				t.Error("No security profile validator provided")
				perExtOK[ext] = false
			}

		default:
			t.Fatal("Unexpected output file")
		}
	}

	for ext, found := range perExtOK {
		if !found {
			t.Fatalf("Missing or wrong `%s`, out of: %v", ext, outputFiles)
		}
	}
}
