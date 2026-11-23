// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package utils

import (
	"encoding/base64"
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
		_, pwd, err := imagePullCredential(index, registries, usernames, passwords)
		return pwd, err
	}).(pulumi.StringOutput)
	username := e.ImagePullPassword().ApplyT(func(passwords string) (string, error) {
		user, _, err := imagePullCredential(index, registries, usernames, passwords)
		return user, err
	}).(pulumi.StringOutput)

	return &helm.RepositoryOptsArgs{
		Username: username,
		Password: credential,
	}
}

// imagePullCredential returns the username and decoded password of the registry at position index
// in the comma-separated lists of the imagePull parameters. It fails when a list is too short.
func imagePullCredential(index int, registries, usernames, passwords string) (string, string, error) {
	userList := strings.Split(usernames, ",")
	if index >= len(userList) {
		return "", "", fmt.Errorf("no image pull username for registry %d of %q", index, registries)
	}
	passwordList := strings.Split(passwords, ",")
	if index >= len(passwordList) {
		return "", "", fmt.Errorf("no image pull password for registry %d of %q", index, registries)
	}
	password, err := decodeImagePullPassword(passwordList[index])
	if err != nil {
		return "", "", err
	}
	return userList[index], password, nil
}

// decodeImagePullPassword returns the image pull password, base64-decoding it when it is
// prefixed with "b64=".
func decodeImagePullPassword(password string) (string, error) {
	encoded, ok := strings.CutPrefix(password, "b64=")
	if !ok {
		return password, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("failed to base64 decode the image pull password: %w", err)
	}
	return string(decoded), nil
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
