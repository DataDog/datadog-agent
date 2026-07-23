// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package sbomtargets

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// DockerComposeManifest runs every SBOM target image as a long-lived Docker
// container so each image stays resident in the Docker daemon (and is marked
// InUse) for the Agent to scan. It is the standalone-Docker counterpart of
// K8sAppDefinition and uses the same Targets, so the SBOMs are identical.
//
// `tail -f /dev/null` (not `sleep infinity`) is used as the entrypoint because
// it keeps the container alive on every target including the busybox-based
// golang:alpine image, where `sleep infinity` is unsupported.
func DockerComposeManifest() docker.ComposeInlineManifest {
	var sb strings.Builder
	sb.WriteString("version: \"3.9\"\nservices:\n")
	for _, t := range Targets {
		fmt.Fprintf(&sb, "  %s:\n", t.Name)
		fmt.Fprintf(&sb, "    image: %s\n", t.Image)
		fmt.Fprintf(&sb, "    container_name: %s\n", t.Name)
		sb.WriteString("    entrypoint: [\"tail\", \"-f\", \"/dev/null\"]\n")
		sb.WriteString("    restart: unless-stopped\n")
	}
	return docker.ComposeInlineManifest{
		Name:    "sbom-targets",
		Content: pulumi.String(sb.String()),
	}
}
