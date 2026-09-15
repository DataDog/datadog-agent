// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubelet

package gpupodresources

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
)

func newTestChecker(t *testing.T, probe func(context.Context) error) *checker {
	t.Helper()

	cfg := config.NewMockWithOverrides(t, map[string]any{
		"kubernetes_kubelet_podresources_socket": "/var/lib/kubelet/pod-resources/kubelet.sock",
	})
	host, _ := hostnamemock.NewMock("node-a")
	return &checker{
		cfg:   cfg,
		host:  host,
		probe: probe,
	}
}

func TestCheckerRun(t *testing.T) {
	t.Run("reports unavailable PodResources API", func(t *testing.T) {
		checker := newTestChecker(t, func(context.Context) error {
			return errors.New("permission denied")
		})

		reports, err := checker.Run()

		require.NoError(t, err)
		require.Len(t, reports, 1)
		assert.Equal(t, IssueName, reports[0].IssueName)
		assert.Equal(t, hostIssueID("node-a"), reports[0].IssueID)
		assert.Equal(t, "permission denied", reports[0].Context[contextKeyError])
		assert.Equal(t, "/var/lib/kubelet/pod-resources/kubelet.sock", reports[0].Context[contextKeySocketPath])
	})

	t.Run("reports no issue when PodResources API is available", func(t *testing.T) {
		checker := newTestChecker(t, func(context.Context) error {
			return nil
		})

		reports, err := checker.Run()

		require.NoError(t, err)
		assert.Empty(t, reports)
	})
}

func TestHostIssueID(t *testing.T) {
	id := hostIssueID("node-a")

	assert.True(t, strings.HasPrefix(id, IssueID+":"))
	assert.Equal(t, id, hostIssueID("node-a"))
	assert.NotEqual(t, id, hostIssueID("node-b"))
}

func TestModuleGate(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_PORT", "443")

	cfg := config.NewMockWithOverrides(t, map[string]any{
		"gpu.enabled": true,
	})
	host, _ := hostnamemock.NewMock("node-a")
	module := NewModule(issues.ModuleDeps{Config: cfg, Hostname: host}).(*gpuPodResourcesModule)
	module.checker.probe = func(context.Context) error { return errors.New("unavailable") }

	reports, err := module.BuiltInPeriodicHealthCheck().Fn()
	require.NoError(t, err)
	require.Len(t, reports, 1)

	cfg.SetInTest("gpu.enabled", false)
	reports, err = module.BuiltInPeriodicHealthCheck().Fn()
	require.NoError(t, err)
	assert.Empty(t, reports)
}
