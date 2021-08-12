// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package checks

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"

	assert "github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic/fake"
	kscheme "k8s.io/client-go/kubernetes/scheme"
)

var scheme = kscheme.Scheme

func init() {
	schemeBuilder := runtime.NewSchemeBuilder(addKnownTypes)
	schemeBuilder.AddToScheme(scheme)
}

type MyObj struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec MyObjSpec `json:"spec,omitempty"`
}

type MyObjSpec struct {
	StringAttribute string                 `json:"stringAttribute,omitempty"`
	BoolAttribute   bool                   `json:"boolAttribute,omitempty"`
	ListAttribute   []interface{}          `json:"listAttribute,omitempty"`
	StructAttribute map[string]interface{} `json:"structAttribute,omitempty"`
}

type MyObjList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MyObj
}

func (in *MyObj) DeepCopyInto(out *MyObj) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
}

func (in *MyObj) DeepCopy() *MyObj {
	if in == nil {
		return nil
	}
	out := new(MyObj)
	in.DeepCopyInto(out)
	return out
}

func (in *MyObj) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

func (in *MyObjList) DeepCopy() *MyObjList {
	if in == nil {
		return nil
	}
	out := new(MyObjList)
	in.DeepCopyInto(out)
	return out
}

func (in *MyObjList) DeepCopyInto(out *MyObjList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]MyObj, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *MyObjList) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(schema.GroupVersion{Group: "mygroup.com", Version: "v1"},
		&MyObj{},
		&MyObjList{},
	)
	return nil
}

type kubeApiserverFixture struct {
	name         string
	resource     compliance.Resource
	objects      []runtime.Object
	expectReport *compliance.Report
}

func newMyObj(namespace, name, uid string) *MyObj {
	return &MyObj{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MyObj",
			APIVersion: "mygroup.com/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(uid),
		},
		Spec: MyObjSpec{
			StringAttribute: "foo",
			BoolAttribute:   true,
			ListAttribute:   []interface{}{"listFoo", "listBar"},
			StructAttribute: map[string]interface{}{
				"name": "nestedFoo",
			},
		},
	}
}

func (f *kubeApiserverFixture) run(t *testing.T) {
	t.Helper()

	assert := assert.New(t)

	env := &mocks.Env{}
	env.On("MaxEventsPerRun").Return(30).Maybe()

	defer env.AssertExpectations(t)

	kubeClient := fake.NewSimpleDynamicClient(scheme, f.objects...)
	env.On("KubeClient").Return(kubeClient)

	kubeCheck, err := newResourceCheck(env, "rule-id", f.resource)
	assert.NoError(err)

	reports := kubeCheck.check(env)
	assert.Equal(f.expectReport.Passed, reports[0].Passed)
	assert.Equal(f.expectReport.Data, reports[0].Data)
	assert.Equal(f.expectReport.Resource, reports[0].Resource)
	if f.expectReport.Error != nil {
		assert.EqualError(reports[0].Error, f.expectReport.Error.Error())
	}
}

func TestKubeApiserverCheck(t *testing.T) {
	tests := []kubeApiserverFixture{
		{
			name: "List case no ns",
			resource: compliance.Resource{
				BaseResource: compliance.BaseResource{
					KubeApiserver: &compliance.KubernetesResource{
						Group:     "mygroup.com",
						Version:   "v1",
						Kind:      "myobjs",
						Namespace: "",
						APIRequest: compliance.KubernetesAPIRequest{
							Verb: "list",
						},
					},
				},
				Condition: `kube.resource.jq(".spec.stringAttribute") == "foo"`,
			},
			objects: []runtime.Object{
				newMyObj("testns", "dummy1", "100"),
			},
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					compliance.KubeResourceFieldName:      "dummy1",
					compliance.KubeResourceFieldNamespace: "testns",
					compliance.KubeResourceFieldKind:      "MyObj",
					compliance.KubeResourceFieldVersion:   "v1",
					compliance.KubeResourceFieldGroup:     "mygroup.com",
				},
				Resource: compliance.ReportResource{
					ID:   "100",
					Type: "kube_myobj",
				},
			},
		},
		{
			name: "List case with ns",
			resource: compliance.Resource{
				BaseResource: compliance.BaseResource{
					KubeApiserver: &compliance.KubernetesResource{
						Group:     "mygroup.com",
						Version:   "v1",
						Kind:      "myobjs",
						Namespace: "testns",
						APIRequest: compliance.KubernetesAPIRequest{
							Verb: "list",
						},
					},
				},
				Condition: `kube.resource.jq(".spec.stringAttribute") != "foo"`,
			},
			objects: []runtime.Object{
				newMyObj("testns", "dummy1", "102"),
				newMyObj("testns2", "dummy1", "103"),
			},
			expectReport: &compliance.Report{
				Passed: false,
				Data: event.Data{
					compliance.KubeResourceFieldName:      "dummy1",
					compliance.KubeResourceFieldNamespace: "testns",
					compliance.KubeResourceFieldKind:      "MyObj",
					compliance.KubeResourceFieldVersion:   "v1",
					compliance.KubeResourceFieldGroup:     "mygroup.com",
				},
				Resource: compliance.ReportResource{
					ID:   "102",
					Type: "kube_myobj",
				},
			},
		},
		{
			name: "List case multiple matches",
			resource: compliance.Resource{
				BaseResource: compliance.BaseResource{
					KubeApiserver: &compliance.KubernetesResource{
						Group:     "mygroup.com",
						Version:   "v1",
						Kind:      "myobjs",
						Namespace: "testns",
						APIRequest: compliance.KubernetesAPIRequest{
							Verb: "list",
						},
					},
				},
				Condition: `kube.resource.jq(".spec.stringAttribute") == "foo"`,
			},
			objects: []runtime.Object{
				newMyObj("testns", "dummy1", "104"),
				newMyObj("testns", "dummy2", "105"),
				newMyObj("testns2", "dummy1", "106"),
			},
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					compliance.KubeResourceFieldName:      "dummy1",
					compliance.KubeResourceFieldNamespace: "testns",
					compliance.KubeResourceFieldKind:      "MyObj",
					compliance.KubeResourceFieldVersion:   "v1",
					compliance.KubeResourceFieldGroup:     "mygroup.com",
				},
				Resource: compliance.ReportResource{
					ID:   "104",
					Type: "kube_myobj",
				},
			},
		},
		{
			name: "Get case",
			resource: compliance.Resource{
				BaseResource: compliance.BaseResource{
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
				},
				Condition: `kube.resource.jq(".spec.stringAttribute") == "foo"`,
			},
			objects: []runtime.Object{
				newMyObj("testns", "dummy1", "107"),
				newMyObj("testns2", "dummy1", "108"),
			},
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					compliance.KubeResourceFieldName:      "dummy1",
					compliance.KubeResourceFieldNamespace: "testns",
					compliance.KubeResourceFieldKind:      "MyObj",
					compliance.KubeResourceFieldVersion:   "v1",
					compliance.KubeResourceFieldGroup:     "mygroup.com",
				},
				Resource: compliance.ReportResource{
					ID:   "107",
					Type: "kube_myobj",
				},
			},
		},
		{
			name: "Get case all type of args",
			resource: compliance.Resource{
				BaseResource: compliance.BaseResource{
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
				},
				Condition: `kube.resource.jq(".spec.structAttribute.name") == "nestedFoo" && kube.resource.jq(".spec.boolAttribute") == "true" && kube.resource.jq(".spec.listAttribute.[0]") == "listFoo"`,
			},
			objects: []runtime.Object{
				newMyObj("testns", "dummy1", "109"),
				newMyObj("testns", "dummy2", "110"),
			},
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					compliance.KubeResourceFieldName:      "dummy1",
					compliance.KubeResourceFieldNamespace: "testns",
					compliance.KubeResourceFieldKind:      "MyObj",
					compliance.KubeResourceFieldVersion:   "v1",
					compliance.KubeResourceFieldGroup:     "mygroup.com",
				},
				Resource: compliance.ReportResource{
					ID:   "109",
					Type: "kube_myobj",
				},
			},
		},
		{
			name: "Error case object not found",
			resource: compliance.Resource{
				BaseResource: compliance.BaseResource{
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
				},
				Condition: `kube.resource.jq(".spec.stringAttribute") == "foo"`,
			},
			objects: []runtime.Object{
				newMyObj("testns", "dummy2", "111"),
			},
			expectReport: &compliance.Report{
				Passed: false,
				Error:  errors.New(`unable to get Kube resource:'mygroup.com/v1, Resource=myobjs', ns:'testns' name:'dummy1', err: myobjs.mygroup.com "dummy1" not found`),
			},
		},
		{
			name: "Error case property does not exist",
			resource: compliance.Resource{
				BaseResource: compliance.BaseResource{
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
				},
				Condition: `kube.resource.jq(".spec.DoesNotExist") == "foo"`,
			},
			objects: []runtime.Object{
				newMyObj("testns", "dummy1", "112"),
			},
			expectReport: &compliance.Report{
				Passed: false,
				Data: event.Data{
					compliance.KubeResourceFieldName:      "dummy1",
					compliance.KubeResourceFieldNamespace: "testns",
					compliance.KubeResourceFieldKind:      "MyObj",
					compliance.KubeResourceFieldVersion:   "v1",
					compliance.KubeResourceFieldGroup:     "mygroup.com",
				},
				Resource: compliance.ReportResource{
					ID:   "112",
					Type: "kube_myobj",
				},
			},
		},
		{
			name: "Error case attribute syntax is wrong",
			resource: compliance.Resource{
				BaseResource: compliance.BaseResource{
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
				},
				Condition: `kube.resource.jq(".spec[@@@]") == "foo"`,
			},
			objects: []runtime.Object{
				newMyObj("testns", "dummy1", "113"),
			},
			expectReport: &compliance.Report{
				Passed: false,
				Error:  errors.New(`1:1: call to "kube.resource.jq()" failed: unexpected token "@"`),
			},
		},
		{
			name: "List with json query selectors",
			resource: compliance.Resource{
				BaseResource: compliance.BaseResource{
					KubeApiserver: &compliance.KubernetesResource{
						Group:     "mygroup.com",
						Version:   "v1",
						Kind:      "myobjs",
						Namespace: "",
						APIRequest: compliance.KubernetesAPIRequest{
							Verb: "list",
						},
					},
				},
				Condition: `kube.resource.namespace != "testns2" || kube.resource.jq(".spec.stringAttribute") == "foo"`,
			},
			objects: []runtime.Object{
				newMyObj("testns", "dummy1", "114"),
				newMyObj("testns2", "dummy1", "115"),
			},
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					compliance.KubeResourceFieldName:      "dummy1",
					compliance.KubeResourceFieldNamespace: "testns",
					compliance.KubeResourceFieldKind:      "MyObj",
					compliance.KubeResourceFieldVersion:   "v1",
					compliance.KubeResourceFieldGroup:     "mygroup.com",
				},
				Resource: compliance.ReportResource{
					ID:   "114",
					Type: "kube_myobj",
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
