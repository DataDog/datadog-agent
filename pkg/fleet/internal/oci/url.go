// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oci

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/env"
)

// PackageURL returns the package URL for the given package and version. Env is used to determine the registry.
func PackageURL(env *env.Env, pkg string, version string) string {
	var url string
	switch env.Site {
	case "datad0g.com":
		url = fmt.Sprintf("docker.io/datadog/%s-package-dev:%s", strings.TrimPrefix(pkg, "datadog-"), version)
	default:
		url = fmt.Sprintf("gcr.io/datadoghq/%s-package:%s", strings.TrimPrefix(pkg, "datadog-"), version)
	}
	return "oci://" + applyRegistryOverride(env, pkg, url)
}

func applyRegistryOverride(env *env.Env, pkg string, url string) string {
	registry := env.RegistryOverride
	_, hasPackageOverride := env.RegistryOverrideByPackage[pkg]
	if hasPackageOverride {
		registry = env.RegistryOverrideByPackage[pkg]
	}
	if registry == "" {
		return url
	}
	if !strings.HasSuffix(registry, "/") {
		registry += "/"
	}
	split := strings.Split(url, "/")
	return registry + split[len(split)-1]
}
