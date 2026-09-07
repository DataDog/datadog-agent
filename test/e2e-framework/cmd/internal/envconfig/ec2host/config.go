// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ec2host defines the EC2 input contract shared by the CLI and executor.
// Keep this package data-only: never import a Pulumi scenario here.
package ec2host

import "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"

// Config describes the host, not the provisioned connection or Agent.
type Config struct {
	OS           string `yaml:"os" config:"required" enum:"ubuntu-22.04,ubuntu-24.04" example:"ubuntu-22.04" description:"Operating system to provision."`
	Arch         string `yaml:"arch" default:"amd64" enum:"amd64,arm64" description:"VM CPU architecture."`
	InstanceType string `yaml:"instance-type,omitempty" description:"Optional EC2 instance type; must match the architecture."`
}

// Schema is shared by both process boundaries. Semantic rules, when needed,
// implement configschema.Validator[Config] and are passed to Must here once.
var Schema = configschema.Must[Config]()
