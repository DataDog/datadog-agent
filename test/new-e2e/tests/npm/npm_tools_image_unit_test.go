// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

// Plain unit tests of the apps-npm-tools image resolution.
// They need no infrastructure and run in the Bazel test jobs (see BUILD.bazel).

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
)

func TestInternalNPMToolsImage(t *testing.T) {
	tests := []struct {
		name              string
		imagePullRegistry string
		want              string
	}{
		{"no registry", "", ""},
		{"single registry", "669783387624.dkr.ecr.us-east-1.amazonaws.com", "669783387624.dkr.ecr.us-east-1.amazonaws.com/apps-npm-tools:" + apps.Version},
		{"registry list", "669783387624.dkr.ecr.us-east-1.amazonaws.com,us-central1-docker.pkg.dev", "669783387624.dkr.ecr.us-east-1.amazonaws.com/apps-npm-tools:" + apps.Version},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, internalNPMToolsImage(tt.imagePullRegistry))
		})
	}
}
