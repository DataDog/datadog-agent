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
)

func init() { suite.Register("CustomResources", customResources) }

const customResourceDefinitions = "apiextensions.k8s.io/v1/customresourcedefinitions"

// customResources runs the tests of custom resource selection and of CRDs installed after the check started.
func customResources(t *testing.T, s *suite.Suite) {
	s.RunTests(t,
		// A check configured with custom resource collectors only collects them.
		suite.Test{
			Name: "CRDCollectorsOnly",
			Setup: suite.Setup{
				Instance: "collectors: []\ncrd_collectors:\n  - " + strings.Join(customResourceNames(suite.Setup{}), "\n  - ") + "\n",
			},
			Steps: []suite.Step{
				{
					Name:   "first run",
					Expect: suite.Live(suite.CustomResources),
				},
			},
		},
		// The check collects the built-in custom resources whose CRDs are installed after it started.
		suite.Test{
			Name:  "CRDInstalledAfterStart",
			Setup: suite.Setup{Undiscovered: customResourceNames(suite.Setup{})},
			Steps: []suite.Step{
				{
					Name:   "first run",
					Expect: suite.Live(suite.Collected),
				},
				{
					Name: "install the CRD of every custom resource",
					Change: func(t *testing.T, env *suite.Env) {
						env.ServeUndiscovered()
						for _, o := range suite.CustomResources(env) {
							env.Cluster.Add(t, crdOf(o))
						}
					},
					Expect: func(t *testing.T, env *suite.Env) suite.Payloads {
						want := suite.Live(suite.CustomResources)(t, env)
						for _, o := range suite.CustomResources(env) {
							want = want.With(env.PayloadsOf(t, customResourceDefinitions, crdOf(o), false))
						}
						return want
					},
				},
			},
		},
	)
}

// customResourceNames returns the full names of the custom resource fixtures that the configuration serves.
func customResourceNames(setup suite.Setup) []string {
	var names []string
	for _, resource := range suite.Resources() {
		if resource.IsCustomResource() {
			names = append(names, resource.FixtureName(suite.ServedVersion(resource, setup)))
		}
	}
	return names
}

// crdOf returns the established CRD of a custom resource fixture.
func crdOf(o *suite.Object) *apiextensionsv1.CustomResourceDefinition {
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
