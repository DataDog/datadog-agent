// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
package checks

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"

	"github.com/stretchr/testify/assert"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

type kubeApiserverFixture struct {
	name     string
	check    kubeApiserverCheck
	objects  []runtime.Object
	expKV    []compliance.KVMap
	expError error
}

func newUnstructured(apiVersion, kind, namespace, name string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata": map[string]interface{}{
				"namespace": namespace,
				"name":      name,
			},
			"spec": spec,
		},
	}
}

func newDummyObject(namespace, name string) *unstructured.Unstructured {
	// Unstructured is only compatible with string, float, int, bool, []interface{}, or map[string]interface{} children.
	// In practice, int/float do not work
	return newUnstructured("mygroup.com/v1", "MyObj", namespace, name, map[string]interface{}{
		"stringAttribute": "foo",
		"boolAttribute":   true,
		"listAttribute":   []interface{}{"listFoo", "listBar"},
		"structAttribute": map[string]interface{}{
			"name": "nestedFoo",
		},
	})
}

func (f *kubeApiserverFixture) run(t *testing.T) {
	t.Helper()

	reporter := f.check.reporter.(*mocks.Reporter)
	f.check.kubeClient = fake.NewSimpleDynamicClient(runtime.NewScheme(), f.objects...)

	expectedCalls := len(f.expKV)
	for _, kv := range f.expKV {
		reporter.On(
			"Report",
			newTestRuleEvent(
				[]string{"check_kind:kubeapiserver"},
				kv,
			),
		).Once()
	}

	err := f.check.Run()
	reporter.AssertNumberOfCalls(t, "Report", expectedCalls)
	assert.Equal(t, f.expError, err)
}

func TestKubeApiserverCheck(t *testing.T) {
	tests := []kubeApiserverFixture{
		{
			name: "List case no ns",
			check: kubeApiserverCheck{
				baseCheck: newTestBaseCheck(&mocks.Reporter{}, checkKindKubeApiserver),
				kubeResource: compliance.KubernetesResource{
					Group:     "mygroup.com",
					Version:   "v1",
					Kind:      "myobjs",
					Namespace: "",
					APIRequest: compliance.KubernetesAPIRequest{
						Verb: "list",
					},
					Report: compliance.Report{
						{
							Kind:     compliance.PropertyKindJSONQuery,
							Property: ".spec.stringAttribute",
							As:       "attr1",
						},
					},
				},
			},
			objects: []runtime.Object{
				newDummyObject("testns", "dummy1"),
			},
			expKV: []compliance.KVMap{
				{
					kubeResourceNameKey:      "dummy1",
					kubeResourceNamespaceKey: "testns",
					kubeResourceKindKey:      "MyObj",
					kubeResourceVersionKey:   "v1",
					kubeResourceGroupKey:     "mygroup.com",
					"attr1":                  "foo",
				},
			},
		},
		{
			name: "List case with ns",
			check: kubeApiserverCheck{
				baseCheck: newTestBaseCheck(&mocks.Reporter{}, checkKindKubeApiserver),
				kubeResource: compliance.KubernetesResource{
					Group:     "mygroup.com",
					Version:   "v1",
					Kind:      "myobjs",
					Namespace: "testns",
					APIRequest: compliance.KubernetesAPIRequest{
						Verb: "list",
					},
					Report: compliance.Report{
						{
							Kind:     compliance.PropertyKindJSONQuery,
							Property: ".spec.stringAttribute",
							As:       "attr1",
						},
					},
				},
			},
			objects: []runtime.Object{
				newDummyObject("testns", "dummy1"),
				newDummyObject("testns2", "dummy1"),
			},
			expKV: []compliance.KVMap{
				{
					kubeResourceNameKey:      "dummy1",
					kubeResourceNamespaceKey: "testns",
					kubeResourceKindKey:      "MyObj",
					kubeResourceVersionKey:   "v1",
					kubeResourceGroupKey:     "mygroup.com",
					"attr1":                  "foo",
				},
			},
		},
		{
			name: "List case multiple matches",
			check: kubeApiserverCheck{
				baseCheck: newTestBaseCheck(&mocks.Reporter{}, checkKindKubeApiserver),
				kubeResource: compliance.KubernetesResource{
					Group:     "mygroup.com",
					Version:   "v1",
					Kind:      "myobjs",
					Namespace: "testns",
					APIRequest: compliance.KubernetesAPIRequest{
						Verb: "list",
					},
					Report: compliance.Report{
						{
							Kind:     compliance.PropertyKindJSONQuery,
							Property: ".spec.stringAttribute",
							As:       "attr1",
						},
					},
				},
			},
			objects: []runtime.Object{
				newDummyObject("testns", "dummy1"),
				newDummyObject("testns", "dummy2"),
				newDummyObject("testns2", "dummy1"),
			},
			expKV: []compliance.KVMap{
				{
					kubeResourceNameKey:      "dummy1",
					kubeResourceNamespaceKey: "testns",
					kubeResourceKindKey:      "MyObj",
					kubeResourceVersionKey:   "v1",
					kubeResourceGroupKey:     "mygroup.com",
					"attr1":                  "foo",
				},
				{
					kubeResourceNameKey:      "dummy2",
					kubeResourceNamespaceKey: "testns",
					kubeResourceKindKey:      "MyObj",
					kubeResourceVersionKey:   "v1",
					kubeResourceGroupKey:     "mygroup.com",
					"attr1":                  "foo",
				},
			},
		},
		{
			name: "Get case",
			check: kubeApiserverCheck{
				baseCheck: newTestBaseCheck(&mocks.Reporter{}, checkKindKubeApiserver),
				kubeResource: compliance.KubernetesResource{
					Group:     "mygroup.com",
					Version:   "v1",
					Kind:      "myobjs",
					Namespace: "testns",
					APIRequest: compliance.KubernetesAPIRequest{
						Verb:         "get",
						ResourceName: "dummy1",
					},
					Report: compliance.Report{
						{
							Kind:     compliance.PropertyKindJSONQuery,
							Property: ".spec.stringAttribute",
							As:       "attr1",
						},
					},
				},
			},
			objects: []runtime.Object{
				newDummyObject("testns", "dummy1"),
				newDummyObject("testns2", "dummy1"),
			},
			expKV: []compliance.KVMap{
				{
					kubeResourceNameKey:      "dummy1",
					kubeResourceNamespaceKey: "testns",
					kubeResourceKindKey:      "MyObj",
					kubeResourceVersionKey:   "v1",
					kubeResourceGroupKey:     "mygroup.com",
					"attr1":                  "foo",
				},
			},
		},
		{
			name: "Get case all type of args",
			check: kubeApiserverCheck{
				baseCheck: newTestBaseCheck(&mocks.Reporter{}, checkKindKubeApiserver),
				kubeResource: compliance.KubernetesResource{
					Group:     "mygroup.com",
					Version:   "v1",
					Kind:      "myobjs",
					Namespace: "testns",
					APIRequest: compliance.KubernetesAPIRequest{
						Verb:         "get",
						ResourceName: "dummy1",
					},
					Report: compliance.Report{
						{
							Kind:     compliance.PropertyKindJSONQuery,
							Property: ".spec.structAttribute.name",
							As:       "attr1",
						},
						{
							Kind:     compliance.PropertyKindJSONQuery,
							Property: ".spec.boolAttribute",
							As:       "attr2",
						},
						{
							Kind:     compliance.PropertyKindJSONQuery,
							Property: ".spec.listAttribute.[0]",
							As:       "attr3",
						},
					},
				},
			},
			objects: []runtime.Object{
				newDummyObject("testns", "dummy1"),
				newDummyObject("testns", "dummy2"),
			},
			expKV: []compliance.KVMap{
				{
					kubeResourceNameKey:      "dummy1",
					kubeResourceNamespaceKey: "testns",
					kubeResourceKindKey:      "MyObj",
					kubeResourceVersionKey:   "v1",
					kubeResourceGroupKey:     "mygroup.com",
					"attr1":                  "nestedFoo",
					"attr2":                  "true",
					"attr3":                  "listFoo",
				},
			},
		},
		{
			name: "Error case object not found",
			check: kubeApiserverCheck{
				baseCheck: newTestBaseCheck(&mocks.Reporter{}, checkKindKubeApiserver),
				kubeResource: compliance.KubernetesResource{
					Group:     "mygroup.com",
					Version:   "v1",
					Kind:      "myobjs",
					Namespace: "testns",
					APIRequest: compliance.KubernetesAPIRequest{
						Verb:         "get",
						ResourceName: "dummy1",
					},
					Report: compliance.Report{
						{
							Kind:     compliance.PropertyKindJSONQuery,
							Property: ".spec.structAttribute.name",
							As:       "attr1",
						},
					},
				},
			},
			objects: []runtime.Object{
				newDummyObject("testns", "dummy2"),
			},
			expError: fmt.Errorf("unable to get Kube resource:'mygroup.com/v1, Resource=myobjs', ns:'testns' name:'dummy1', err: myobjs.mygroup.com \"dummy1\" not found"),
		},
		{
			name: "Error case one property does not exist",
			check: kubeApiserverCheck{
				baseCheck: newTestBaseCheck(&mocks.Reporter{}, checkKindKubeApiserver),
				kubeResource: compliance.KubernetesResource{
					Group:     "mygroup.com",
					Version:   "v1",
					Kind:      "myobjs",
					Namespace: "testns",
					APIRequest: compliance.KubernetesAPIRequest{
						Verb:         "get",
						ResourceName: "dummy1",
					},
					Report: compliance.Report{
						{
							Kind:     compliance.PropertyKindJSONQuery,
							Property: ".spec.structAttribute.name",
							As:       "attr1",
						},
						{
							Kind:     compliance.PropertyKindJSONQuery,
							Property: ".spec.IdoNotExist",
							As:       "attr2",
						},
						{
							Kind:     compliance.PropertyKindJSONQuery,
							Property: ".spec.listAttribute.[0]",
							As:       "attr3",
						},
					},
				},
			},
			objects: []runtime.Object{
				newDummyObject("testns", "dummy1"),
				newDummyObject("testns", "dummy2"),
			},
			expKV: []compliance.KVMap{
				{
					kubeResourceNameKey:      "dummy1",
					kubeResourceNamespaceKey: "testns",
					kubeResourceKindKey:      "MyObj",
					kubeResourceVersionKey:   "v1",
					kubeResourceGroupKey:     "mygroup.com",
					"attr1":                  "nestedFoo",
					"attr3":                  "listFoo",
				},
			},
		},
		{
			name: "Error case attribute syntax is wrong",
			check: kubeApiserverCheck{
				baseCheck: newTestBaseCheck(&mocks.Reporter{}, checkKindKubeApiserver),
				kubeResource: compliance.KubernetesResource{
					Group:     "mygroup.com",
					Version:   "v1",
					Kind:      "myobjs",
					Namespace: "testns",
					APIRequest: compliance.KubernetesAPIRequest{
						Verb:         "get",
						ResourceName: "dummy1",
					},
					Report: compliance.Report{
						{
							Kind:     compliance.PropertyKindJSONQuery,
							Property: ".spec[@@@]",
							As:       "attr1",
						},
					},
				},
			},
			objects: []runtime.Object{
				newDummyObject("testns", "dummy1"),
			},
			expError: fmt.Errorf("unable to report field: '.spec[@@@]' for kubernetes object 'mygroup.com/v1, Kind=MyObj / testns / dummy1' - json query error: 1:7: unexpected token \"@\" (expected \"]\")"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t)
		})
	}
}

func TestKubeApiserverFilters(t *testing.T) {
	// TODO: Find a way to make fake dynamicClient work with label/field selectors
	tests := []kubeApiserverFixture{
		{
			name: "List with json query selectors",
			check: kubeApiserverCheck{
				baseCheck: newTestBaseCheck(&mocks.Reporter{}, checkKindKubeApiserver),
				kubeResource: compliance.KubernetesResource{
					Group:     "mygroup.com",
					Version:   "v1",
					Kind:      "myobjs",
					Namespace: "",
					APIRequest: compliance.KubernetesAPIRequest{
						Verb: "list",
					},
					Report: compliance.Report{
						{
							Kind:     compliance.PropertyKindJSONQuery,
							Property: ".spec.stringAttribute",
							As:       "attr1",
						},
					},
					Filter: []compliance.Filter{
						{
							Include: &compliance.Condition{
								Kind:      compliance.ConditionKindJSONQuery,
								Property:  ".metadata.name",
								Value:     "dummy1",
								Operation: compliance.OpEqual,
							},
						},
						{
							Include: &compliance.Condition{
								Kind:      compliance.ConditionKindJSONQuery,
								Property:  ".spec.boolAttribute",
								Value:     "true",
								Operation: compliance.OpEqual,
							},
						},
						{
							Exclude: &compliance.Condition{
								Kind:      compliance.ConditionKindJSONQuery,
								Property:  ".metadata.name",
								Value:     "dummy2",
								Operation: compliance.OpEqual,
							},
						},
						{
							Exclude: &compliance.Condition{
								Kind:      compliance.ConditionKindJSONQuery,
								Property:  ".metadata.foo.bar",
								Operation: compliance.OpExists,
							},
						},
					},
				},
			},
			objects: []runtime.Object{
				newDummyObject("testns", "dummy1"),
				newDummyObject("testns", "dummy2"),
			},
			expKV: []compliance.KVMap{
				{
					kubeResourceNameKey:      "dummy1",
					kubeResourceNamespaceKey: "testns",
					kubeResourceKindKey:      "MyObj",
					kubeResourceVersionKey:   "v1",
					kubeResourceGroupKey:     "mygroup.com",
					"attr1":                  "foo",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t)
		})
	}
}
