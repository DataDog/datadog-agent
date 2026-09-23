// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helm

import (
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
)

// ImageArtifact is installer-owned delivery evidence, separate from routing
// capabilities. A Docker-daemon image ID is NOT automatically a repository digest
// (the underlying object varies across Docker storage backends).
// For kind-loaded images the artifact owner must deliver and verify a unique
// immutable Tag matching LocalImageID. Registry images use RepositoryDigest.
// This structure is not user YAML: the artifact adapter owns its verification.
type ImageArtifact struct {
	Repository       string
	Tag              string
	LocalImageID     string
	RepositoryDigest string
}

var imageDigest = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var artifactRepository = regexp.MustCompile(`^[a-zA-Z0-9.-]+(:[0-9]+)?/[a-zA-Z0-9/._-]+$`)
var artifactTag = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9._-]*$`)

func (a ImageArtifact) Validate() error {
	if !artifactRepository.MatchString(a.Repository) {
		return fmt.Errorf("artifact requires a registry-qualified repository")
	}
	if a.RepositoryDigest != "" {
		if !imageDigest.MatchString(a.RepositoryDigest) || a.LocalImageID != "" || a.Tag != "" {
			return fmt.Errorf("registry artifact requires only a genuine repository digest, not a Docker image ID/tag")
		}
	} else if !imageDigest.MatchString(a.LocalImageID) || !artifactTag.MatchString(a.Tag) {
		return fmt.Errorf("local image artifact requires a verified Docker image ID and unique deliverable tag")
	}
	return nil
}

func validateProducerProfile(p Params) error {
	if p.Profile == nil {
		// Release installs render version-agnostic routing: the chart's standard
		// datadog.* values. A released AgentVersion is not evidence, so none is
		// required — but a dev image without capability evidence is a silent
		// unsupported-combination risk, and is rejected honestly.
		if p.Image != nil {
			return fmt.Errorf("dev image artifacts require producer capability evidence")
		}
		return nil
	}
	if err := p.Profile.Require(receivers.CoreAgent, receivers.TraceAgent, receivers.ProcessAgent, receivers.ClusterChecksRunner); err != nil {
		return err
	}
	if p.Image == nil {
		return fmt.Errorf("producer capability evidence requires its verified image artifact")
	}
	return p.Image.Validate()
}

func applyImageArtifact(values map[string]interface{}, image *ImageArtifact) {
	if image == nil {
		return
	}
	fields := map[string]interface{}{"repository": image.Repository, "digest": image.RepositoryDigest}
	if image.RepositoryDigest == "" {
		fields["tag"] = image.Tag
		fields["pullPolicy"] = "Never"
	}
	for _, role := range []string{"agents", "clusterChecksRunner"} {
		mergeMaps(values, map[string]interface{}{role: map[string]interface{}{"image": fields}})
	}
}
