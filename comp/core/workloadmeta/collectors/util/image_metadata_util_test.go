// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package util contains utility functions for workload metadata collectors
package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRepoDigest(t *testing.T) {
	for _, tc := range []struct {
		repoDigest string
		registry   string
		repository string
		digest     string
	}{
		{
			repoDigest: "727006795293.dkr.ecr.us-east-1.amazonaws.com/spidly@sha256:fce79f86f7a3b9c660112da8484a8f5858a7da9e703892ba04c6f045da714300",
			registry:   "727006795293.dkr.ecr.us-east-1.amazonaws.com",
			repository: "spidly",
			digest:     "sha256:fce79f86f7a3b9c660112da8484a8f5858a7da9e703892ba04c6f045da714300",
		},
		{
			repoDigest: "docker.io/library/docker@sha256:b813c414ee36b8a2c44b45295698df6bdc3bdee4a435481dbb892e1b44e09d3b",
			registry:   "docker.io",
			repository: "library/docker",
			digest:     "sha256:b813c414ee36b8a2c44b45295698df6bdc3bdee4a435481dbb892e1b44e09d3b",
		},
		{
			repoDigest: "eu.gcr.io/datadog-staging/logs-event-store-api@sha256:747bd4fc36f3f263b5dcb9df907b98489a4cb46d636c223dc29f1fb7f9405070",
			registry:   "eu.gcr.io",
			repository: "datadog-staging/logs-event-store-api",
			digest:     "sha256:747bd4fc36f3f263b5dcb9df907b98489a4cb46d636c223dc29f1fb7f9405070",
		},
		{
			repoDigest: "registry.ddbuild.io/apm-integrations-testing/handmade/postgres",
			registry:   "registry.ddbuild.io",
			repository: "apm-integrations-testing/handmade/postgres",
			digest:     "",
		},
		{
			repoDigest: "registry.ddbuild.io/apm-integrations-testing/handmade/postgres@sha256:747bd4fc36f3f263b5dcb9df907b98489a4cb46d636c223dc29f1fb7f9405070",
			registry:   "registry.ddbuild.io",
			repository: "apm-integrations-testing/handmade/postgres",
			digest:     "sha256:747bd4fc36f3f263b5dcb9df907b98489a4cb46d636c223dc29f1fb7f9405070",
		},
		{
			repoDigest: "@sha256:747bd4fc36f3f263b5dcb9df907b98489a4cb46d636c223dc29f1fb7f9405070",
			registry:   "",
			repository: "",
			digest:     "sha256:747bd4fc36f3f263b5dcb9df907b98489a4cb46d636c223dc29f1fb7f9405070",
		},
		{
			repoDigest: "docker.io@sha256:747bd4fc36f3f263b5dcb9df907b98489a4cb46d636c223dc29f1fb7f9405070",
			registry:   "docker.io",
			repository: "",
			digest:     "sha256:747bd4fc36f3f263b5dcb9df907b98489a4cb46d636c223dc29f1fb7f9405070",
		},
	} {
		registry, repository, digest := parseRepoDigest(tc.repoDigest)
		assert.Equal(t, tc.registry, registry)
		assert.Equal(t, tc.repository, repository)
		assert.Equal(t, tc.digest, digest)
	}
}
