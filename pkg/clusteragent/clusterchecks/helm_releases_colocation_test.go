// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func orchestratorConfig(collectors ...string) integration.Config {
	instance := "collectors: [" + strings.Join(collectors, ", ") + "]\n"
	return integration.Config{
		Name:         orchestratorCheckName,
		ClusterCheck: true,
		Instances:    []integration.Data{integration.Data(instance)},
	}
}

func helmConfig() integration.Config {
	return integration.Config{
		Name:         helmCheckName,
		ClusterCheck: true,
		Instances:    []integration.Data{integration.Data("collect_events: true\n")},
	}
}

func TestHelmReleasesColocationGroup(t *testing.T) {
	r := newHelmReleasesColocation(true)

	assert.Equal(t, helmReleasesGroup, r.group(helmConfig()))
	assert.Equal(t, helmReleasesGroup, r.group(orchestratorConfig("pods", "configmaps")))
	assert.Equal(t, helmReleasesGroup, r.group(orchestratorConfig("v1/configmaps")))

	// Orchestrator instances that don't run the ConfigMap collector, and other
	// checks, have no group.
	assert.Empty(t, r.group(orchestratorConfig("pods", "nodes")))
	assert.Empty(t, r.group(orchestratorConfig("apps/v1/configmaps")))
	assert.Empty(t, r.group(integration.Config{Name: orchestratorCheckName}))
	assert.Empty(t, r.group(generateIntegration("other")))
	invalid := integration.Config{Name: orchestratorCheckName, Instances: []integration.Data{integration.Data("collectors: configmaps: [")}}
	assert.Empty(t, r.group(invalid))
}

func TestHelmReleasesColocationFollowsSetting(t *testing.T) {
	configmock.New(t).SetInTest(helmReleasesConfigMapsSetting, true)
	d := newDispatcher(taggerfxmock.SetupFakeTagger(t))
	assert.Equal(t, helmReleasesGroup, d.colocationGroup(helmConfig()))
	assert.Equal(t, helmReleasesGroup, d.colocationGroup(orchestratorConfig("configmaps")))

	// Nothing is colocated when the Helm releases collector is off.
	configmock.New(t).SetInTest(helmReleasesConfigMapsSetting, false)
	d = newDispatcher(taggerfxmock.SetupFakeTagger(t))
	assert.Empty(t, d.colocationGroup(helmConfig()))
	assert.Empty(t, d.colocationGroup(orchestratorConfig("configmaps")))
}
