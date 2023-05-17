// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

package tests

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/scheme"
)

func getMockedKubeClient(t *testing.T, objects ...runtime.Object) dynamic.Interface {
	addKnownTypes := func(scheme *runtime.Scheme) error {
		scheme.AddKnownTypes(schema.GroupVersion{Group: "mygroup.com", Version: "v1"},
			&MyObj{},
			&MyObjList{},
		)
		return nil
	}
	schemeBuilder := runtime.NewSchemeBuilder(addKnownTypes)
	schemeBuilder.AddToScheme(scheme.Scheme)
	return fake.NewSimpleDynamicClient(scheme.Scheme, objects...)
}

func TestKubernetesCluster(t *testing.T) {
	kubeClient := getMockedKubeClient(t,
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				UID:  "my-cluster",
				Name: "kube-system",
			},
		},
		newMyObj("testns1", "dummy1", "100"),
		newMyObj("testns2", "dummy1", "101"),
		newMyObj("testns2", "dummy2", "102"),
		newMyObj("testns3", "dummy1", "103"),
		newMyObj("testns3", "dummy2", "104"),
		newMyObj("testns3", "dummy3", "105"),
	)

	b := NewTestBench(t).WithKubeClient(kubeClient)
	defer b.Run()

	b.
		AddRule("BadNamespace").
		WithScope("kubernetesCluster").
		WithInput(`
- kubeApiserver:
		kind: myobjs
		group: "dd.compliance.com"
		version: v1
		namespace: testnsnotexist
		apiRequest:
			verb: list
	type: array
`).
		WithRego(`
package datadog

import data.datadog as dd
import data.helpers as h

has_key(o, k) {
	_ := o[k]
}

findings[f] {
	has_key(input, "kubernetes")
	count(input.kubernetes) == 0
	f := dd.passed_finding(
		"my_resource_type",
		"my_resource_id",
		{}
	)
}
`).
		AssertPassedEvent(nil)

	b.
		AddRule("OneObject").
		WithScope("kubernetesCluster").
		WithInput(`
- kubeApiserver:
		kind: myobjs
		group: "dd.compliance.com"
		version: v1
		namespace: testns1
		apiRequest:
			verb: list
	type: array
	tag: foo1
`).
		WithRego(`
package datadog

import data.datadog as dd
import data.helpers as h

has_key(o, k) {
	_ := o[k]
}

valid(p) {
	p.group = "dd.compliance.com"
	p.kind = "MyObj"
	p.name = "dummy1"
	p.version = "v1"
	p.namespace = "testns1"
	p.resource.Object.apiVersion = "dd.compliance.com/v1"
	p.resource.Object.kind = "MyObj"
	p.resource.Object.spec.boolAttribute = true
	p.resource.Object.spec.listAttribute[0] = "listFoo"
	p.resource.Object.spec.listAttribute[1] = "listBar"
	p.resource.Object.spec.stringAttribute = "foo"
	p.resource.Object.spec.structAttribute.name = "nestedFoo"
}

findings[f] {
	input.context.kubernetes_cluster = "my-cluster"
	count(input.foo1) = 1
	[obj | obj := input.foo1[_]; valid(obj)]
	f := dd.passed_finding(
		"my_resource_type",
		"my_resource_id",
		{}
	)
}
`).
		AssertPassedEvent(nil)

	b.
		AddRule("MultiObjects").
		WithScope("kubernetesCluster").
		WithInput(`
- kubeApiserver:
		kind: myobjs
		group: "dd.compliance.com"
		version: v1
		namespace: testns3
		apiRequest:
			verb: list
	type: array
	tag: foo1
`).
		WithRego(`
package datadog

import data.datadog as dd
import data.helpers as h

has_key(o, k) {
	_ := o[k]
}

valid(p) {
	p.group = "dd.compliance.com"
	p.kind = "MyObj"
	p.name = "dummy1"
	p.version = "v1"
	p.namespace = "testns3"
	p.resource.Object.apiVersion = "dd.compliance.com/v1"
	p.resource.Object.kind = "MyObj"
	p.resource.Object.spec.boolAttribute = true
	p.resource.Object.spec.listAttribute[0] = "listFoo"
	p.resource.Object.spec.listAttribute[1] = "listBar"
	p.resource.Object.spec.stringAttribute = "foo"
	p.resource.Object.spec.structAttribute.name = "nestedFoo"
}

findings[f] {
	input.context.kubernetes_cluster = "my-cluster"
	count(input.foo1) = 3
	[obj | obj := input.foo1[_]; valid(obj)]
	f := dd.passed_finding(
		"my_resource_type",
		"my_resource_id",
		{}
	)
}
`).
		AssertPassedEvent(nil)
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

func newMyObj(namespace, name, uid string) runtime.Object {
	return &MyObj{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MyObj",
			APIVersion: "dd.compliance.com/v1",
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
