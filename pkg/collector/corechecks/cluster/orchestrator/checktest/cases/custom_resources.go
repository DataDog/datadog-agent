// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package cases

import (
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/checktest/suite"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/registry"
)

func init() { suite.Register("CustomResources", customResources) }

const customResourceDefinitions = "apiextensions.k8s.io/v1/customresourcedefinitions"

// customResources runs the tests of custom resource selection and of CRDs installed after the check started.
func customResources(t *testing.T, cs *suite.Suite) {
	cs.RunCases(t,
		// A check configured with custom resource collectors only collects them.
		suite.Case{
			Name: "CRDCollectorsOnly",
			Configure: func(cfg *suite.ScenarioConfig) {
				cfg.Instance = "collectors: []\ncrd_collectors:\n  - " + strings.Join(customResourceNames(*cfg), "\n  - ") + "\n"
			},
			Steps: []suite.Step{{Name: "first run", Want: func(t *testing.T, sc *suite.Scenario) suite.ExpectedPayloads {
				return sc.LiveOf(t, customResourceObjects(t, sc))
			}}},
		},
		// The check collects the built-in custom resources whose CRDs are installed after it started.
		suite.Case{
			Name:      "CRDInstalledAfterStart",
			Configure: func(cfg *suite.ScenarioConfig) { cfg.Undiscovered = customResourceNames(*cfg) },
			Steps: []suite.Step{
				suite.FirstRun,
				{
					Name: "install the CRD of every custom resource",
					Act: func(t *testing.T, sc *suite.Scenario) {
						cfg := sc.Cfg
						cfg.Undiscovered = nil
						sc.Expected = suite.ExpectedObjects(sc.Objects, cfg)
						for _, o := range customResourceObjects(t, sc) {
							sc.Mocks.Cluster.Add(t, crdOf(o))
						}
						sc.Mocks.Cluster.Resources = suite.CustomResourceDiscovery(sc.Objects, cfg)
					},
					Want: func(t *testing.T, sc *suite.Scenario) suite.ExpectedPayloads {
						crds := suite.ExpectedPayloads{}
						for _, o := range customResourceObjects(t, sc) {
							crds = crds.With(sc.PayloadsOf(t, customResourceDefinitions, crdOf(o), false))
						}
						return sc.LiveOf(t, customResourceObjects(t, sc)).With(crds)
					},
				},
			},
		},
	)
}

// customResourceNames returns the full names of the custom resource fixtures that the configuration serves.
func customResourceNames(cfg suite.ScenarioConfig) []string {
	var names []string
	for _, resource := range registry.Resources() {
		if resource.IsCustomResource() {
			names = append(names, resource.FixtureName(suite.ServedVersion(resource, cfg)))
		}
	}
	return names
}

// customResourceObjects returns the custom resource fixtures of the scenario.
func customResourceObjects(t *testing.T, sc *suite.Scenario) []suite.ScenarioObject {
	var objects []suite.ScenarioObject
	for _, name := range customResourceNames(sc.Cfg) {
		objects = append(objects, sc.Object(t, name))
	}
	return objects
}

// crdOf returns the established CRD of a custom resource fixture.
func crdOf(o suite.ScenarioObject) *apiextensionsv1.CustomResourceDefinition {
	parts := strings.Split(o.FullName, "/")
	group, version := parts[0], parts[1]
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: o.APIResource.Name + "." + group, UID: types.UID(o.FullName + "-crd"), ResourceVersion: "1"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group:    group,
			Names:    apiextensionsv1.CustomResourceDefinitionNames{Plural: o.APIResource.Name, Kind: o.APIResource.Kind},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{Name: version, Served: true, Storage: true}},
		},
		Status: apiextensionsv1.CustomResourceDefinitionStatus{Conditions: []apiextensionsv1.CustomResourceDefinitionCondition{
			{Type: apiextensionsv1.Established, Status: apiextensionsv1.ConditionTrue},
		}},
	}
}
