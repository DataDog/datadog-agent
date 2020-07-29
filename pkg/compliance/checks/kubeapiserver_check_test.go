// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
package checks

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"

	assert "github.com/stretchr/testify/require"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

type kubeApiserverFixture struct {
	name         string
	resource     compliance.Resource
	objects      []runtime.Object
	expectReport *report
	expectError  error
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

	assert := assert.New(t)

	env := &mocks.Env{}
	defer env.AssertExpectations(t)

	kubeClient := fake.NewSimpleDynamicClient(runtime.NewScheme(), f.objects...)
	env.On("KubeClient").Return(kubeClient)

	expr, err := eval.ParseIterable(f.resource.Condition)
	assert.NoError(err)

	report, err := checkKubeapiserver(env, "rule-id", f.resource, expr)
	assert.Equal(f.expectReport, report)
	if f.expectError != nil {
		assert.EqualError(err, f.expectError.Error())
	}
}

func TestKubeApiserverCheck(t *testing.T) {
	tests := []kubeApiserverFixture{
		{
			name: "List case no ns",
			resource: compliance.Resource{
				KubeApiserver: &compliance.KubernetesResource{
					Group:     "mygroup.com",
					Version:   "v1",
					Kind:      "myobjs",
					Namespace: "",
					APIRequest: compliance.KubernetesAPIRequest{
						Verb: "list",
					},
				},
				Condition: `kube.resource.jq(".spec.stringAttribute") == "foo"`,
			},
			objects: []runtime.Object{
				newDummyObject("testns", "dummy1"),
			},
			expectReport: &report{
				passed: true,
				data: event.Data{
					kubeResourceFieldName:      "dummy1",
					kubeResourceFieldNamespace: "testns",
					kubeResourceFieldKind:      "MyObj",
					kubeResourceFieldVersion:   "v1",
					kubeResourceFieldGroup:     "mygroup.com",
				},
			},
		},
		{
			name: "List case with ns",
			resource: compliance.Resource{
				KubeApiserver: &compliance.KubernetesResource{

					Group:     "mygroup.com",
					Version:   "v1",
					Kind:      "myobjs",
					Namespace: "testns",
					APIRequest: compliance.KubernetesAPIRequest{
						Verb: "list",
					},
				},
				Condition: `kube.resource.jq(".spec.stringAttribute") != "foo"`,
			},
			objects: []runtime.Object{
				newDummyObject("testns", "dummy1"),
				newDummyObject("testns2", "dummy1"),
			},
			expectReport: &report{
				passed: false,
				data: event.Data{
					kubeResourceFieldName:      "dummy1",
					kubeResourceFieldNamespace: "testns",
					kubeResourceFieldKind:      "MyObj",
					kubeResourceFieldVersion:   "v1",
					kubeResourceFieldGroup:     "mygroup.com",
				},
			},
		},
		{
			name: "List case multiple matches",
			resource: compliance.Resource{
				KubeApiserver: &compliance.KubernetesResource{
					Group:     "mygroup.com",
					Version:   "v1",
					Kind:      "myobjs",
					Namespace: "testns",
					APIRequest: compliance.KubernetesAPIRequest{
						Verb: "list",
					},
				},
				Condition: `kube.resource.jq(".spec.stringAttribute") == "foo"`,
			},
			objects: []runtime.Object{
				newDummyObject("testns", "dummy1"),
				newDummyObject("testns", "dummy2"),
				newDummyObject("testns2", "dummy1"),
			},
			expectReport: &report{
				passed: true,
				data: event.Data{
					kubeResourceFieldName:      "dummy1",
					kubeResourceFieldNamespace: "testns",
					kubeResourceFieldKind:      "MyObj",
					kubeResourceFieldVersion:   "v1",
					kubeResourceFieldGroup:     "mygroup.com",
				},
			},
		},
		{
			name: "Get case",
			resource: compliance.Resource{
				KubeApiserver: &compliance.KubernetesResource{
					Group:     "mygroup.com",
					Version:   "v1",
					Kind:      "myobjs",
					Namespace: "testns",
					APIRequest: compliance.KubernetesAPIRequest{
						Verb:         "get",
						ResourceName: "dummy1",
					},
				},
				Condition: `kube.resource.jq(".spec.stringAttribute") == "foo"`,
			},
			objects: []runtime.Object{
				newDummyObject("testns", "dummy1"),
				newDummyObject("testns2", "dummy1"),
			},
			expectReport: &report{
				passed: true,
				data: event.Data{
					kubeResourceFieldName:      "dummy1",
					kubeResourceFieldNamespace: "testns",
					kubeResourceFieldKind:      "MyObj",
					kubeResourceFieldVersion:   "v1",
					kubeResourceFieldGroup:     "mygroup.com",
				},
			},
		},
		{
			name: "Get case all type of args",
			resource: compliance.Resource{
				KubeApiserver: &compliance.KubernetesResource{
					Group:     "mygroup.com",
					Version:   "v1",
					Kind:      "myobjs",
					Namespace: "testns",
					APIRequest: compliance.KubernetesAPIRequest{
						Verb:         "get",
						ResourceName: "dummy1",
					},
				},
				Condition: `kube.resource.jq(".spec.structAttribute.name") == "nestedFoo" && kube.resource.jq(".spec.boolAttribute") == "true" && kube.resource.jq(".spec.listAttribute.[0]") == "listFoo"`,
			},
			objects: []runtime.Object{
				newDummyObject("testns", "dummy1"),
				newDummyObject("testns", "dummy2"),
			},
			expectReport: &report{
				passed: true,
				data: event.Data{
					kubeResourceFieldName:      "dummy1",
					kubeResourceFieldNamespace: "testns",
					kubeResourceFieldKind:      "MyObj",
					kubeResourceFieldVersion:   "v1",
					kubeResourceFieldGroup:     "mygroup.com",
				},
			},
		},
		{
			name: "Error case object not found",
			resource: compliance.Resource{
				KubeApiserver: &compliance.KubernetesResource{
					Group:     "mygroup.com",
					Version:   "v1",
					Kind:      "myobjs",
					Namespace: "testns",
					APIRequest: compliance.KubernetesAPIRequest{
						Verb:         "get",
						ResourceName: "dummy1",
					},
				},
				Condition: `kube.resource.jq(".spec.stringAttribute") == "foo"`,
			},
			objects: []runtime.Object{
				newDummyObject("testns", "dummy2"),
			},
			expectError: errors.New(`unable to get Kube resource:'mygroup.com/v1, Resource=myobjs', ns:'testns' name:'dummy1', err: myobjs.mygroup.com "dummy1" not found`),
		},
		{
			name: "Error case property does not exist",
			resource: compliance.Resource{
				KubeApiserver: &compliance.KubernetesResource{
					Group:     "mygroup.com",
					Version:   "v1",
					Kind:      "myobjs",
					Namespace: "testns",
					APIRequest: compliance.KubernetesAPIRequest{
						Verb:         "get",
						ResourceName: "dummy1",
					},
				},
				Condition: `kube.resource.jq(".spec.DoesNotExist") == "foo"`,
			},
			objects: []runtime.Object{
				newDummyObject("testns", "dummy1"),
			},
			expectReport: &report{
				passed: false,
				data: event.Data{
					kubeResourceFieldName:      "dummy1",
					kubeResourceFieldNamespace: "testns",
					kubeResourceFieldKind:      "MyObj",
					kubeResourceFieldVersion:   "v1",
					kubeResourceFieldGroup:     "mygroup.com",
				},
			},
		},
		{
			name: "Error case attribute syntax is wrong",
			resource: compliance.Resource{
				KubeApiserver: &compliance.KubernetesResource{
					Group:     "mygroup.com",
					Version:   "v1",
					Kind:      "myobjs",
					Namespace: "testns",
					APIRequest: compliance.KubernetesAPIRequest{
						Verb:         "get",
						ResourceName: "dummy1",
					},
				},
				Condition: `kube.resource.jq(".spec[@@@]") == "foo"`,
			},
			objects: []runtime.Object{
				newDummyObject("testns", "dummy1"),
			},
			expectError: errors.New(`1:1: call to "kube.resource.jq()" failed: 1:7: unexpected token "@" (expected "]")`),
		},
		{
			name: "List with json query selectors",
			resource: compliance.Resource{
				KubeApiserver: &compliance.KubernetesResource{
					Group:     "mygroup.com",
					Version:   "v1",
					Kind:      "myobjs",
					Namespace: "",
					APIRequest: compliance.KubernetesAPIRequest{
						Verb: "list",
					},
				},
				Condition: `kube.resource.namespace != "testns2" || kube.resource.jq(".spec.stringAttribute") == "foo"`,
			},
			objects: []runtime.Object{
				newDummyObject("testns", "dummy1"),
				newDummyObject("testns2", "dummy1"),
			},
			expectReport: &report{
				passed: true,
				data: event.Data{
					kubeResourceFieldName:      "dummy1",
					kubeResourceFieldNamespace: "testns",
					kubeResourceFieldKind:      "MyObj",
					kubeResourceFieldVersion:   "v1",
					kubeResourceFieldGroup:     "mygroup.com",
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
