// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fakeintake

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	"github.com/DataDog/datadog-agent/test/fakeintake/version"
)

// ImageURL returns the fakeintake image for the given registry-qualified image
// name: the FakeintakeImageOverride runner parameter if set, else <image>:<version.Tag>.
func ImageURL(image string) string {
	if override, err := runner.GetProfile().ParamStore().GetWithDefault(parameters.FakeintakeImageOverride, ""); err == nil && override != "" {
		return override
	}
	return image + ":" + version.Tag
}
