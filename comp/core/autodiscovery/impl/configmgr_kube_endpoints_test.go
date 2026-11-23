// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver && test

package autodiscoveryimpl

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
)

func matchProvider(provider string) func(integration.Config) bool {
	return func(config integration.Config) bool {
		return config.Provider == provider
	}
}

func newEndpointAnnotationPrecedenceTemplate(endpointID, provider, source, version string) integration.Config {
	return integration.Config{
		Name:          "redisdb",
		Provider:      provider,
		Source:        source,
		ADIdentifiers: []string{endpointID},
		Instances:     []integration.Data{integration.Data("version: " + version)},
	}
}

func (suite *ReconcilingConfigManagerSuite) TestEndpointAnnotationPrecedenceTransitions() {
	newService := func() *listeners.KubeEndpointService {
		return listeners.CreateDummyKubeEndpoint("myservice", "default", nil, nil)
	}
	endpointID := newService().GetServiceID()
	newTemplate := func(provider, source, version string) integration.Config {
		return newEndpointAnnotationPrecedenceTemplate(endpointID, provider, source, version)
	}

	annotationV1 := newTemplate(names.KubeEndpointSlices, "kube_endpoints:default/myservice", "annotation-v1")
	crV1 := newTemplate(names.KubeEndpointSlicesCR, "datadoginstrumentation:default/cr1", "cr-v1")
	require.NotEqual(suite.T(), annotationV1.Digest(), crV1.Digest())

	for _, tc := range []struct {
		name  string
		first integration.Config
		last  integration.Config
	}{
		{name: "CR arrives first", first: crV1, last: annotationV1},
		{name: "annotation arrives first", first: annotationV1, last: crV1},
	} {
		suite.Run(tc.name, func() {
			cm := suite.factory(suite.captureChanges)
			cm.processNewService(newService())
			require.True(suite.T(), suite.changes.IsEmpty())

			cm.processNewConfig(tc.first)
			assert.Len(suite.T(), suite.changes.Schedule, 1)
			assert.Empty(suite.T(), suite.changes.Unschedule)

			cm.processNewConfig(tc.last)
			if tc.last.Provider == names.KubeEndpointSlices {
				assertConfigsMatch(suite.T(), suite.changes.Schedule, matchProvider(names.KubeEndpointSlices))
				assertConfigsMatch(suite.T(), suite.changes.Unschedule, matchProvider(names.KubeEndpointSlicesCR))
			} else {
				require.True(suite.T(), suite.changes.IsEmpty())
			}
			assertLoadedConfigsMatch(suite.T(), cm, matchProvider(names.KubeEndpointSlices))

			// Removing the overridden CR must leave the annotation running.
			cm.processDelConfigs([]integration.Config{crV1})
			require.True(suite.T(), suite.changes.IsEmpty())
			assertLoadedConfigsMatch(suite.T(), cm, matchProvider(names.KubeEndpointSlices))

			// Restoring the CR while the annotation exists has no scheduling effect;
			// removing the annotation then hands scheduling back to the CR.
			cm.processNewConfig(crV1)
			require.True(suite.T(), suite.changes.IsEmpty())
			cm.processDelConfigs([]integration.Config{annotationV1})
			assertConfigsMatch(suite.T(), suite.changes.Schedule, matchProvider(names.KubeEndpointSlicesCR))
			assertConfigsMatch(suite.T(), suite.changes.Unschedule, matchProvider(names.KubeEndpointSlices))
			assertLoadedConfigsMatch(suite.T(), cm, matchProvider(names.KubeEndpointSlicesCR))
		})
	}
}

func (suite *ReconcilingConfigManagerSuite) TestEndpointAnnotationPrecedenceUpdates() {
	service := listeners.CreateDummyKubeEndpoint("myservice", "default", nil, nil)
	newTemplate := func(provider, source, version string) integration.Config {
		return newEndpointAnnotationPrecedenceTemplate(service.GetServiceID(), provider, source, version)
	}
	annotationV1 := newTemplate(names.KubeEndpointSlices, "kube_endpoints:default/myservice", "annotation-v1")
	crV1 := newTemplate(names.KubeEndpointSlicesCR, "datadoginstrumentation:default/cr1", "cr-v1")

	cm := suite.factory(suite.captureChanges)
	cm.processNewService(service)
	require.True(suite.T(), suite.changes.IsEmpty())
	cm.processNewConfig(crV1)
	cm.processNewConfig(annotationV1)

	crV2 := newTemplate(names.KubeEndpointSlicesCR, "datadoginstrumentation:default/cr1", "cr-v2")
	cm.processDelConfigs([]integration.Config{crV1})
	require.True(suite.T(), suite.changes.IsEmpty())
	cm.processNewConfig(crV2)
	require.True(suite.T(), suite.changes.IsEmpty())
	assertLoadedConfigsMatch(suite.T(), cm, matchProvider(names.KubeEndpointSlices))

	annotationV2 := newTemplate(names.KubeEndpointSlices, "kube_endpoints:default/myservice", "annotation-v2")
	cm.processDelConfigs([]integration.Config{annotationV1})
	assertConfigsMatch(suite.T(), suite.changes.Schedule, matchProvider(names.KubeEndpointSlicesCR))
	assertConfigsMatch(suite.T(), suite.changes.Unschedule, matchProvider(names.KubeEndpointSlices))
	require.Contains(suite.T(), string(suite.changes.Schedule[0].Instances[0]), "version: cr-v2")
	cm.processNewConfig(annotationV2)
	assertConfigsMatch(suite.T(), suite.changes.Schedule, matchProvider(names.KubeEndpointSlices))
	assertConfigsMatch(suite.T(), suite.changes.Unschedule, matchProvider(names.KubeEndpointSlicesCR))
	require.Contains(suite.T(), string(suite.changes.Schedule[0].Instances[0]), "version: annotation-v2")
	assertLoadedConfigsMatch(suite.T(), cm, matchProvider(names.KubeEndpointSlices))

	cm.processDelConfigs([]integration.Config{crV2})
	require.True(suite.T(), suite.changes.IsEmpty())
	assertLoadedConfigsMatch(suite.T(), cm, matchProvider(names.KubeEndpointSlices))
}
