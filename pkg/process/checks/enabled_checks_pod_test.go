// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet && orchestrator

package checks

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
)

func TestPodCheck(t *testing.T) {
	config.SetFeatures(t, config.Kubernetes)

	t.Run("enabled", func(t *testing.T) {
		// Resets the cluster name so that it isn't cached during the call to `getEnabledChecks()`
		clustername.ResetClusterName()
		defer clustername.ResetClusterName()

		cfg := config.Mock(t)
		cfg.Set("orchestrator_explorer.enabled", true)
		cfg.Set("cluster_name", "test")

		enabledChecks := getEnabledChecks(t, cfg, config.MockSystemProbe(t))
		assertContainsCheck(t, enabledChecks, PodCheckName)
	})

	t.Run("disabled", func(t *testing.T) {
		clustername.ResetClusterName()
		defer clustername.ResetClusterName()

		cfg := config.Mock(t)
		cfg.Set("orchestrator_explorer.enabled", false)

		enabledChecks := getEnabledChecks(t, cfg, config.MockSystemProbe(t))
		assertNotContainsCheck(t, enabledChecks, PodCheckName)
	})
}
