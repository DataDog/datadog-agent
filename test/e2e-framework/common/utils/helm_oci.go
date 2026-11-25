// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package utils

import (
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
)

// HelmOCIRepositoryOpts returns the Helm repository options to pull the OCI chart chartRef
// (oci://<host>/...) with the credentials of the imagePull parameters, when <host> is one of
// their registries (e.g. the internal ECR in CI). It returns nil otherwise, for public charts.
func HelmOCIRepositoryOpts(e config.Env, chartRef string) helm.RepositoryOptsPtrInput {
	registries := e.ImagePullRegistry()
	index := ociChartRegistryIndex(chartRef, registries)
	if index < 0 {
		return nil
	}

	// A registry without credentials would make Helm pull anonymously and fail with an opaque
	// "unauthorized": fail with the cause instead.
	usernames := e.ImagePullUsername()
	credential := e.ImagePullPassword().ApplyT(func(passwords string) (string, error) {
		c, err := imagePullCredential(index, registries, usernames, passwords)
		return c.Password, err
	}).(pulumi.StringOutput)
	username := e.ImagePullPassword().ApplyT(func(passwords string) (string, error) {
		c, err := imagePullCredential(index, registries, usernames, passwords)
		return c.Username, err
	}).(pulumi.StringOutput)

	return &helm.RepositoryOptsArgs{
		Username: username,
		Password: credential,
	}
}

// imagePullCredential returns the credential of the registry at position index in the
// comma-separated lists of the imagePull parameters, with the same validation as DockerConfigJSON.
func imagePullCredential(index int, registries, usernames, passwords string) (ImagePullCredential, error) {
	credentials, err := ParseImagePullCredentials(registries, usernames, passwords)
	if err != nil {
		return ImagePullCredential{}, err
	}
	if index < 0 || index >= len(credentials) {
		return ImagePullCredential{}, fmt.Errorf("no image pull credential for registry %d of %q", index, registries)
	}
	return credentials[index], nil
}

// ociChartRegistryIndex returns the position of the registry of chartRef (oci://<host>/...) in the
// comma-separated list registries, or -1.
func ociChartRegistryIndex(chartRef, registries string) int {
	ref, ok := strings.CutPrefix(chartRef, "oci://")
	if !ok || registries == "" {
		return -1
	}
	host, _, _ := strings.Cut(ref, "/")
	for i, registry := range strings.Split(registries, ",") {
		if strings.TrimSpace(registry) == host {
			return i
		}
	}
	return -1
}
