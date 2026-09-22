// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eks defines the EKS input contract shared by the CLI and executor.
// Keep this package data-only: never import a Pulumi scenario here.
package eks

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"
)

// Config describes the cluster topology, not the provisioned connection or Agent.
// First-release boundaries (qa-plans/pending/qa-e2ectl-eks-scenario-plan.md):
// AL2023 Linux and optional Windows Server Core 2022 managed nodes, one node
// per enabled group, no Fargate profile, released Agent versions only.
type Config struct {
	Linux   bool   `yaml:"linux" default:"true" description:"Managed node group of AL2023 Linux nodes (amd64); the Cluster Agent and system components run on Linux."`
	Windows bool   `yaml:"windows,omitempty" default:"false" description:"Add a managed node group of Windows Server Core 2022 nodes (amd64) alongside the Linux nodes."`
	Version string `yaml:"version" default:"1.34" enum:"1.32,1.33,1.34" description:"Kubernetes control-plane version; the per-OS node AMI release is resolved from the framework's nodes-version table."`
}

// Rules is the shared topology contract: an explicitly disabled group is
// never silently re-enabled, and Windows never implies its Linux dependency.
type Rules struct{}

func (Rules) Validate(c Config) error {
	if c.Windows && !c.Linux {
		return fmt.Errorf("windows: Windows nodes require Linux nodes — the Cluster Agent and other system components run on Linux; keep linux enabled alongside windows")
	}
	if !c.Linux && !c.Windows {
		return fmt.Errorf("linux: at least one node group must stay enabled — a control-plane-only cluster is not a usable QA target")
	}
	return nil
}

// Schema is shared by both process boundaries, topology rules included.
var Schema = configschema.Must[Config](Rules{})
