// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package util

import (
	"strings"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// SHA256 is the prefix used by containerd for the repo digest
	SHA256 = "@sha256:"
)

// ExtractRepoDigestFromImage extracts the repoDigest from workloadmeta store if it exists and unique
// the format of repoDigest is "registry/repo@sha256:digest", the format of return value is "sha256:digest"
func ExtractRepoDigestFromImage(imageID string, imageRegistry string, store workloadmeta.Component) string {
	existingImgMetadata, err := store.GetImage(imageID)
	if err == nil {
		// If there is only one repoDigest, return it
		if len(existingImgMetadata.RepoDigests) == 1 {
			_, _, digest := parseRepoDigest(existingImgMetadata.RepoDigests[0])
			return digest
		}
		// If there are multiple repoDigests, we should find the one that matches the current container's registry
		for _, repoDigest := range existingImgMetadata.RepoDigests {
			registry, _, digest := parseRepoDigest(repoDigest)
			if registry == imageRegistry {
				return digest
			}
		}
		log.Debugf("cannot get matched registry in repodigests for image %s", imageID)
	} else {
		// TODO: we should handle the case when the image metadata is not found in the store
		// For example, there could be a rare race condition when collection of image metadata is not finished yet
		// In this case, we can either call containerd or docker directly or get it from knownImages in the local cache
		log.Debugf("cannot get image metadata for image %s: %s", imageID, err)
	}
	return ""
}

// parseRepoDigest extracts registry, repository, digest from repoDigest (in the format of "registry/repo@sha256:digest")
func parseRepoDigest(repoDigest string) (string, string, string) {
	var registry, repository, digest string
	imageName := repoDigest

	digestStart := strings.Index(repoDigest, SHA256)
	if digestStart != -1 {
		digest = repoDigest[digestStart+len("@"):]
		imageName = repoDigest[:digestStart]
	}

	registryEnd := strings.Index(imageName, "/")
	// e.g <registry>/<repository>
	if registryEnd != -1 {
		registry = imageName[:registryEnd]
		repository = imageName[registryEnd+len("/"):]
		// e.g <registry>
	} else {
		registry = imageName
	}

	return registry, repository, digest
}
