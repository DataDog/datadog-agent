// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package byohost builds StaticStackProvisioner-compatible JSON descriptors for a
// bring-your-own host: a local machine or an arbitrary box reachable over SSH, with no
// Pulumi program and no cloud provider involved. See
// priv_notes/macos-e2e-byo-host-plan.md for the design this implements.
package byohost

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	oscomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
)

// WriteHostDescriptor writes a StaticStackProvisioner-compatible JSON file describing a
// single pre-existing host reachable at addr:port over SSH, for use with
// provisioners.NewStaticStackProvisioner[environments.Host]. The written file always sets
// cloudProvider to "local" (components.CloudProviderLocal): key resolution then falls back
// to "" from the param store, and the SSH client falls back to the local ssh-agent -- no
// AWS/Azure/GCP key needs to be configured for this host.
func WriteHostDescriptor(path string, addr string, port int, username string, descriptor oscomp.Descriptor) error {
	return WriteHostDescriptorWithFakeIntake(path, addr, port, username, descriptor, "", 0)
}

// WriteHostDescriptorWithFakeIntake is WriteHostDescriptor plus a "fakeIntake" key pointing
// at a fakeintake reachable at fakeIntakeHost:fakeIntakePort. Pass an empty fakeIntakeHost to
// omit the key, matching WriteHostDescriptor's behavior.
func WriteHostDescriptorWithFakeIntake(path string, addr string, port int, username string, descriptor oscomp.Descriptor, fakeIntakeHost string, fakeIntakePort uint32) error {
	hostOutput := remote.HostOutput{
		CloudProvider: components.CloudProviderLocal,
		Address:       addr,
		Port:          port,
		Username:      username,
		OSFamily:      descriptor.Family(),
		OSFlavor:      descriptor.Flavor,
		OSVersion:     descriptor.Version,
		Architecture:  descriptor.Architecture,
	}

	fields := map[string]any{
		"_source":    "byohost.WriteHostDescriptor",
		"remoteHost": hostOutput,
	}

	if fakeIntakeHost != "" {
		fields["fakeIntake"] = fakeintake.FakeintakeOutput{
			Host:   fakeIntakeHost,
			Scheme: "http",
			Port:   fakeIntakePort,
			URL:    fmt.Sprintf("http://%s:%d", fakeIntakeHost, fakeIntakePort),
		}
	}

	data, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling host descriptor: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing host descriptor to %s: %w", path, err)
	}
	return nil
}
