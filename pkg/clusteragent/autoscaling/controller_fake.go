// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package autoscaling

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/dynamic/fake"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	core "k8s.io/client-go/testing"

	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
)

var (
	scheme             = kscheme.Scheme
	alwaysReady        = func() bool { return true }
	noResyncPeriodFunc = func() time.Duration { return 0 }
)

func init() {
	_ = datadoghq.AddToScheme(scheme)
}

// CreateControllerFunc is a function that creates a new controller.
type CreateControllerFunc func(fakeClient *fake.FakeDynamicClient, informer dynamicinformer.DynamicSharedInformerFactory, isLeader func() bool) (*Controller, error)

// ControllerFixture is a fixture to help test the controller.
type ControllerFixture struct {
	t                    *testing.T
	ctx                  context.Context
	createControllerFunc CreateControllerFunc
	gvr                  schema.GroupVersionResource

	// Client to use for the controller.
	Client *fake.FakeDynamicClient
	// Objects to preload into the informer lister.
	InformerObjects []*unstructured.Unstructured
	// Actions expected to happen on the client.
	Actions []core.Action
	// Objects from here preloaded into Fake client.
	Objects []runtime.Object
}

// NewFixture creates a new fixture.
func NewFixture(t *testing.T, gvr schema.GroupVersionResource, newController CreateControllerFunc) *ControllerFixture {
	ctx := context.Background()
	if testDeadline, found := t.Deadline(); found {
		ctx, _ = context.WithDeadline(ctx, testDeadline.Add(-time.Second)) //nolint:govet
	}

	return &ControllerFixture{
		t:                    t,
		ctx:                  ctx,
		gvr:                  gvr,
		createControllerFunc: newController,
		Objects:              []runtime.Object{},
	}
}

func (f *ControllerFixture) newController(leader bool) (*Controller, dynamicinformer.DynamicSharedInformerFactory) {
	f.Client = fake.NewSimpleDynamicClient(scheme, f.Objects...)
	informer := dynamicinformer.NewDynamicSharedInformerFactory(f.Client, noResyncPeriodFunc())

	c, err := f.createControllerFunc(f.Client, informer, getIsLeaderFunction(leader))
	if err != nil {
		return nil, nil
	}
	c.synced = alwaysReady

	for _, metric := range f.InformerObjects {
		err := informer.ForResource(f.gvr).Informer().GetIndexer().Add(metric)
		if err != nil {
			f.t.Errorf("Failed to add object to informer: %v", err)
		}
	}

	return c, informer
}

// RunControllerSync runs the controller sync loop and checks the expected actions.
func (f *ControllerFixture) RunControllerSync(leader bool, objectID string) {
	f.t.Helper()
	controller, informer := f.newController(leader)
	stopCh := make(chan struct{})
	defer close(stopCh)
	informer.Start(stopCh)

	controller.Workqueue.Add(objectID)
	assert.True(f.t, controller.process())

	actions := FilterInformerActions(f.Client.Actions(), f.gvr.Resource)
	for i, action := range actions {
		if len(f.Actions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(actions)-len(f.Actions), actions[i:])
			break
		}

		expectedAction := f.Actions[i]
		CheckAction(f.t, expectedAction, action)
	}

	if len(f.Actions) > len(actions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.Actions)-len(actions), f.Actions[len(actions):])
	}
}

// ExpectCreateAction adds an expected create action.
func (f *ControllerFixture) ExpectCreateAction(obj *unstructured.Unstructured) {
	action := core.NewCreateAction(f.gvr, obj.GetNamespace(), obj)
	f.Actions = append(f.Actions, action)
}

// ExpectDeleteAction adds an expected delete action.
func (f *ControllerFixture) ExpectDeleteAction(ns, name string) {
	action := core.NewDeleteAction(f.gvr, ns, name)
	f.Actions = append(f.Actions, action)
}

// ExpectUpdateAction adds an expected update action.
func (f *ControllerFixture) ExpectUpdateAction(obj *unstructured.Unstructured) {
	action := core.NewUpdateAction(f.gvr, obj.GetNamespace(), obj)
	f.Actions = append(f.Actions, action)
}

// ExpectUpdateStatusAction adds an expected update status action.
func (f *ControllerFixture) ExpectUpdateStatusAction(obj *unstructured.Unstructured) {
	action := core.NewUpdateSubresourceAction(f.gvr, "status", obj.GetNamespace(), obj)
	f.Actions = append(f.Actions, action)
}

func getIsLeaderFunction(leader bool) func() bool {
	return func() bool {
		return leader
	}
}
