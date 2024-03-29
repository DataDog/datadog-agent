// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package runner

import (
	cdx "github.com/CycloneDX/cyclonedx-go"
	trivycore "github.com/aquasecurity/trivy/pkg/sbom/core"
	trivydx "github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	"github.com/distribution/reference"
)

const (
	repoTagPropertyKey    = trivydx.Namespace + trivycore.PropertyRepoTag
	repoDigestPropertyKey = trivydx.Namespace + trivycore.PropertyRepoDigest
)

func appendSBOMRepoMetadata(sbom *cdx.BOM, imageRefTagged reference.NamedTagged, imageRefCanonical reference.Canonical) {
	// reference: https://github.com/aquasecurity/trivy/blob/13f797f885ff007901df7c4b42ecd78604582f5a/pkg/fanal/image/remote.go#L53-L90
	if sbom != nil &&
		sbom.Metadata != nil &&
		sbom.Metadata.Component != nil &&
		sbom.Metadata.Component.Properties != nil {
		props := sbom.Metadata.Component.Properties
		*props = append(*props, cdx.Property{
			Name:  repoTagPropertyKey,
			Value: reference.FamiliarName(imageRefTagged) + ":" + imageRefTagged.Tag(),
		})
		*props = append(*props, cdx.Property{
			Name:  repoDigestPropertyKey,
			Value: reference.FamiliarName(imageRefTagged) + "@" + imageRefCanonical.Digest().String(),
		})
	}
}
