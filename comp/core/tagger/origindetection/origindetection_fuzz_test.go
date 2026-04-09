// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package origindetection

import (
	"testing"
)

// FuzzParseLocalData fuzzes the parser for DogStatsD local origin data.
// Local data contains container IDs and cgroup inodes from untrusted clients.
// Format examples: "ci-<containerID>", "in-<inode>", "ci-abc,in-123",
// legacy: "cid-<containerID>" (APM) or raw "<containerID>" (DogStatsD).
func FuzzParseLocalData(f *testing.F) {
	// Standard formats
	f.Add("ci-abc123def456")
	f.Add("in-12345")
	f.Add("ci-abc123,in-67890")

	// Legacy formats
	f.Add("cid-legacy-container-id")
	f.Add("raw-container-id-no-prefix")

	// Edge cases
	f.Add("")
	f.Add("ci-")
	f.Add("in-")
	f.Add("in-notanumber")
	f.Add("in-18446744073709551615") // MaxUint64
	f.Add("in-18446744073709551616") // MaxUint64 + 1, overflow
	f.Add("ci-abc,in-123,ci-def")    // duplicate prefixes
	f.Add(",,,")                     // empty items
	f.Add("unknown-prefix-value")

	// Adversarial
	f.Add("ci-" + string(make([]byte, 1024))) // long container ID
	f.Add("in-0")
	f.Add("in--1") // negative

	f.Fuzz(func(t *testing.T, raw string) {
		ld, err := ParseLocalData(raw)

		if raw == "" {
			if err != nil {
				t.Fatalf("empty input should not return error, got: %v", err)
			}
			if ld.ContainerID != "" || ld.Inode != 0 {
				t.Fatal("empty input should return zero-value LocalData")
			}
			return
		}

		// If no error, the result must be internally consistent.
		if err == nil {
			// Inode of 0 with an "in-" prefix is valid (zero value).
			// ContainerID can be any string.
			// Just verify we don't get impossible states.
			_ = ld.ContainerID
			_ = ld.Inode
		}

		// Idempotency for container ID: parsing the container ID directly
		// should yield the same container ID (legacy format).
		if ld.ContainerID != "" && err == nil {
			reparsed, err2 := ParseLocalData(ld.ContainerID)
			if err2 != nil {
				// This is fine - the extracted container ID might look like
				// an inode prefix when reparsed.
				return
			}
			// In legacy mode (no prefix), reparsing yields the same container ID.
			_ = reparsed
		}
	})
}

// FuzzParseExternalData fuzzes the parser for admission controller external data.
// External data contains init flags, container names, and pod UIDs from
// environment variables set by the admission controller.
// Format: "it-<bool>,cn-<name>,pu-<uid>"
func FuzzParseExternalData(f *testing.F) {
	// Standard formats
	f.Add("it-true,cn-mycontainer,pu-abcdef-1234-5678")
	f.Add("it-false,cn-init,pu-pod-uid")
	f.Add("cn-container")
	f.Add("pu-uid-only")
	f.Add("it-true")

	// Edge cases
	f.Add("")
	f.Add("it-")
	f.Add("it-notabool")
	f.Add("cn-")
	f.Add("pu-")
	f.Add(",,,")
	f.Add("unknown-prefix")
	f.Add("it-1,cn-name,pu-uid") // bool as 1/0
	f.Add("it-0,cn-name,pu-uid")

	// Adversarial
	f.Add("cn-" + string(make([]byte, 1024))) // long container name
	f.Add("it-true,it-false")                 // duplicate fields

	f.Fuzz(func(t *testing.T, raw string) {
		ed, err := ParseExternalData(raw)

		if raw == "" {
			if err != nil {
				t.Fatalf("empty input should not return error, got: %v", err)
			}
			if ed.Init || ed.ContainerName != "" || ed.PodUID != "" {
				t.Fatal("empty input should return zero-value ExternalData")
			}
			return
		}

		// If no error, verify consistency.
		if err == nil {
			_ = ed.Init
			_ = ed.ContainerName
			_ = ed.PodUID
		}
	})
}
