// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package controllers

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type shutdownWPAClient struct {
	dynamic.Interface
	resource dynamic.NamespaceableResourceInterface
}

func (c shutdownWPAClient) Resource(schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return c.resource
}

type shutdownWPAResource struct {
	dynamic.NamespaceableResourceInterface
	list func(context.Context) (*unstructured.UnstructuredList, error)
}

func (r shutdownWPAResource) List(ctx context.Context, _ metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return r.list(ctx)
}

func TestWPACRDWaitStops(t *testing.T) {
	for _, inflight := range []bool{false, true} {
		t.Run(map[bool]string{false: "retry", true: "inflight"}[inflight], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				calls := 0
				client := shutdownWPAClient{resource: shutdownWPAResource{list: func(ctx context.Context) (*unstructured.UnstructuredList, error) {
					calls++
					if inflight {
						<-ctx.Done()
						return nil, ctx.Err()
					}
					return nil, apierrors.NewNotFound(schema.GroupResource{Group: gvrWPA.Group, Resource: gvrWPA.Resource}, "")
				}}}
				stop := make(chan struct{})
				done := make(chan struct{})
				controller := &autoscalersController{}
				go func() { controller.runWPA(stop, client, nil); close(done) }()
				synctest.Wait()
				require.Equal(t, 1, calls)
				close(stop)
				<-done
				require.False(t, controller.wpaEnabled)
			})
		})
	}
}
