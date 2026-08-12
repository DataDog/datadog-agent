// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import "github.com/DataDog/datadog-agent/pkg/privateactionrunner/artifacts"

// TODO: Remove this hardcoded catalog when PAR receives the authored-script catalog through Remote Config.
const (
	helmAddRepoAction       = "com.datadoghq.authoredscripts.helm.addRepo"
	helmAddRepoArtifactName = "dd-par-scripts-helm-add-repo"
	helmAddRepoVersion      = "0.0.1"
	helmAddRepoDigest       = "sha256:ea7829a6ebdaa464eb4fbfff4c72e6e63176df58a430a4b0b8dfb66f0e57149c"
	helmAddRepoArtifactURL  = "oci://registry.ddbuild.io/dd-authored-scripts/dd-par-scripts-helm-add-repo@" + helmAddRepoDigest
)

func NewStaticCatalog() artifacts.Catalog {
	descriptor := artifacts.Descriptor{
		Name:    helmAddRepoArtifactName,
		Version: helmAddRepoVersion,
		URL:     helmAddRepoArtifactURL,
		Digest:  helmAddRepoDigest,
	}

	platformEntries := make(map[artifacts.Platform]artifacts.Descriptor)
	for _, platform := range staticPlatforms() {
		platformEntries[platform] = descriptor
	}
	return artifacts.NewStaticCatalog(map[string]map[artifacts.Platform]artifacts.Descriptor{
		helmAddRepoAction: platformEntries,
	})
}

func staticPlatforms() []artifacts.Platform {
	return []artifacts.Platform{
		{OS: "linux", Arch: "amd64"},
		{OS: "linux", Arch: "arm64"},
	}
}
