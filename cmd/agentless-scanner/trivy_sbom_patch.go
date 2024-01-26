package main

import (
	cdx "github.com/CycloneDX/cyclonedx-go"
	trivydx "github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	trivydxcore "github.com/aquasecurity/trivy/pkg/sbom/cyclonedx/core"
	"github.com/docker/distribution/reference"
)

const (
	repoTagPropertyKey    = trivydxcore.Namespace + trivydx.PropertyRepoTag
	repoDigestPropertyKey = trivydxcore.Namespace + trivydx.PropertyRepoDigest
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
