// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package workloads describes the environment-agnostic workload declarations
// in the top-level workloads section. The shape is validated here; which
// forms are actually supported is validated by the selected environment
// at prepare time — the same principle as the agent section's installers.
package workloads

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"
)

// Workload is one test application to deploy alongside the Agent.
type Workload struct {
	// App is a named pre-built workload from the framework catalog.
	App string `yaml:"app,omitempty" example:"nginx" description:"Deploy a named test workload from the catalog."`
	// Manifest is inline YAML or a path to a YAML file (multi-document supported).
	Manifest string `yaml:"manifest,omitempty" description:"Inline manifest or a path to a YAML file."`
	// Image is a Docker image reference (container-native environments).
	Image string `yaml:"image,omitempty" description:"Docker image to run as a workload."`
	// Name overrides the derived name (for multi-instance workloads).
	Name string `yaml:"name,omitempty" description:"Override the workload name."`
	// Namespace overrides the target namespace (Kubernetes only).
	Namespace string `yaml:"namespace,omitempty" description:"Kubernetes namespace override."`
}

// Rules validates the shape of each workload entry.
type Rules struct{}

func (Rules) Validate(w Workload) error {
	forms := 0
	for _, set := range []bool{w.App != "", w.Manifest != "", w.Image != ""} {
		if set {
			forms++
		}
	}
	switch forms {
	case 0:
		return fmt.Errorf("exactly one of app, manifest or image is required")
	case 1:
		return nil
	default:
		return fmt.Errorf("app, manifest and image are mutually exclusive")
	}
}

// Schema validates one workload entry.
var Schema = configschema.Must[Workload](Rules{})

// Config holds the decoded list, for File.Workloads.
type Config struct {
	Workloads []Workload `yaml:"workloads,omitempty"`
}
