// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package checktest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/checktest/suite"
)

// TestCollectorSelection checks how the instance configuration selects collectors. The cluster has an object of every
// resource, so the objects sent show the collectors that run.
func TestCollectorSelection(t *testing.T) {
	t.Run("collector_with_its_version", func(t *testing.T) {
		env := suite.Setup(t, suite.WithOlderVersions(), suite.WithCollectors("batch/v1beta1/cronjobs"))

		env.AssertPop(t, "live batch_v1beta1_cronjobs")
	})

	// The check ignores an unknown collector and a repeated one, and runs the others.
	t.Run("unknown_and_repeated_collectors", func(t *testing.T) {
		env := suite.Setup(t, suite.WithCollectors("deployments", "deployments", "unknowns"))

		env.AssertPop(t, "live apps_v1_deployments")
	})

	// A custom resource that the check does not know is collected with crd_collectors.
	t.Run("user_custom_resource", func(t *testing.T) {
		env := suite.Setup(t,
			suite.WithInstance("collectors: []\ncrd_collectors:\n  - stable.example.com/v1/crontabs\n"),
			suite.WithExtraObjects(suite.CronTab("c")))

		env.AssertPop(t, "live c")
	})

	// crd_collectors ignores a built-in resource, which its built-in collector collects, and refuses Secrets.
	t.Run("built_in_resources_in_crd_collectors", func(t *testing.T) {
		env := suite.Setup(t, suite.WithInstance("collectors: []\ncrd_collectors:\n  - apps/v1/deployments\n  - v1/secrets\n"))

		env.AssertPop(t)
	})

	// The check only uses the first 100 crd_collectors.
	t.Run("too_many_crd_collectors", func(t *testing.T) {
		var instance strings.Builder
		instance.WriteString("collectors: []\ncrd_collectors:\n")
		for i := range 100 {
			fmt.Fprintf(&instance, "  - example.com/v1/unknowns%d\n", i)
		}
		instance.WriteString("  - stable.example.com/v1/crontabs\n")
		env := suite.Setup(t, suite.WithInstance(instance.String()), suite.WithExtraObjects(suite.CronTab("c")))

		env.AssertPop(t)
	})
}
