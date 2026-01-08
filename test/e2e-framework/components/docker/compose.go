// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package docker

import "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

type ComposeInlineManifest struct {
	Name    string
	Content pulumi.StringInput
}

type ComposeManifest struct {
	Version  string                            `yaml:"version"`
	Services map[string]ComposeManifestService `yaml:"services"`
}

type ComposeManifestService struct {
	Pid           string         `yaml:"pid,omitempty"`
	Privileged    bool           `yaml:"privileged,omitempty"`
	Ports         []string       `yaml:"ports,omitempty"`
	Image         string         `yaml:"image"`
	ContainerName string         `yaml:"container_name"`
	Volumes       []string       `yaml:"volumes"`
	Environment   map[string]any `yaml:"environment"`
}
