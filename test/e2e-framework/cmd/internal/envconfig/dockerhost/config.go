// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dockerhost defines the Docker-on-EC2 input contract shared by the
// CLI and executor: an EC2 VM with the Docker runtime the framework's
// ec2docker scenario installs. Keep this package data-only: never import a
// Pulumi scenario here.
package dockerhost

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"
)

// Config describes the VM, not the provisioned connection or Agent.
// OS values are the framework's e2e AMIs the docker scenario is built on
// (unlike ec2-host's plain community AMIs — the docker manager expects the
// e2e image tooling).
type Config struct {
	OS           string `yaml:"os" default:"ubuntu-22.04-e2e" enum:"ubuntu-22.04-e2e,ubuntu-24.04-e2e" example:"ubuntu-22.04-e2e" description:"VM operating system (e2e AMI)."`
	Arch         string `yaml:"arch" default:"amd64" enum:"amd64,arm64" description:"VM CPU architecture."`
	InstanceType string `yaml:"instance-type,omitempty" description:"Optional EC2 instance type; must match the architecture."`
}

// Rules is the shared AMI-availability contract: rejecting an unsupported
// OS/architecture pair here keeps the failure before any cloud call, in both
// the CLI and the executor.
type Rules struct{}

func (Rules) Validate(c Config) error {
	if c.Arch == "arm64" && c.OS != "ubuntu-22.04-e2e" {
		return fmt.Errorf("arch: no ARM AMI for %s — arm64 is available on ubuntu-22.04-e2e only", c.OS)
	}
	return nil
}

// Schema is shared by both process boundaries, AMI rules included.
var Schema = configschema.Must[Config](Rules{})
