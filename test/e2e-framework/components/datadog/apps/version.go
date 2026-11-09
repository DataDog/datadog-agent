// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package apps

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
)

const Version = "v0.0.9"

// Image is ImageWithVersion at Version.
func Image(e config.Env, repo string) string {
	return ImageWithVersion(e, repo, Version)
}

// ImageWithVersion returns the image of the app repo at version, from the internal registry of
// e if it has one, from ghcr.io otherwise. Apps use Image, unless they need another version of
// their image than Version.
func ImageWithVersion(e config.Env, repo, version string) string {
	if reg := e.InternalRegistry(); reg != "" && reg != "none" {
		return reg + "/" + repo + ":" + version
	}
	return "ghcr.io/datadog/" + repo + ":" + version
}
