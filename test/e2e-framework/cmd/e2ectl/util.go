// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

package main

import (
	"encoding/json"
	"os/exec"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
)

// readSnapshot reads a snapshot file (Pulumi-free path).
func readSnapshot(path string) (provisioner.RawResources, map[string]json.RawMessage, error) {
	return provisioner.ReadSnapshotFile(path)
}

func jsonUnmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }

func osCommand(name string, args ...string) *exec.Cmd { return exec.Command(name, args...) }
