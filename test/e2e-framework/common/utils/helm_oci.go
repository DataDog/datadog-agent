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

	usernames := strings.Split(e.ImagePullUsername(), ",")
	if index >= len(usernames) {
		return nil
	}
	password := e.ImagePullPassword().ApplyT(func(passwords string) (string, error) {
		list := strings.Split(passwords, ",")
		if index >= len(list) {
			return "", fmt.Errorf("no image pull password for registry %d of %q", index, registries)
		}
		pwd := list[index]
		if strings.HasPrefix(pwd, "b64=") {
			decoded, err := base64.StdEncoding.DecodeString(pwd[4:])
			if err != nil {
				return "", fmt.Errorf("failed to base64 decode the image pull password: %w", err)
			}
			pwd = string(decoded)
		}
		return pwd, nil
	}).(pulumi.StringOutput)

	return &helm.RepositoryOptsArgs{
		Username: pulumi.String(usernames[index]),
		Password: password,
	}
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
