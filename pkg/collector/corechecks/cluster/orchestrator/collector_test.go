/*
 * Unless explicitly stated otherwise all files in this repository are licensed
 * under the Apache License Version 2.0.
 * This product includes software developed at Datadog (https://www.datadoghq.com/).
 * Copyright 2016-2022 Datadog, Inc.
 */

package orchestrator

import (
	"fmt"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/tools/cache"
	fcache "k8s.io/client-go/tools/cache/testing"
)

func TestInformerRunTwiceWillFail(t *testing.T) {
	source := fcache.NewFakeControllerSource()
	for i := 0; i < 1000000; i++ {
		source.Add(&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("pod-%d", i)}})
	}

	informer := cache.NewSharedInformer(source, &v1.Pod{}, 1*time.Second)
	handler := cache.ResourceEventHandlerFuncs{
		// do nothing
	}
	informer.AddEventHandler(handler)

	stop := make(chan struct{})

	// Call informer.Run() twice as they are shared by the same client

	// i.e. orchestrator check init code
	go informer.Run(stop)
	// i.e. indirectly through externalMetrics: apiCl.InformerFactory.Start(ctx.Done())
	go informer.Run(stop)

	// wait a bit until we panic
	time.Sleep(30 * time.Second)
}

func TestInformerRunTwiceWillFailWithFactory(t *testing.T) {
	source := fcache.NewFakeControllerSource()
	for i := 0; i < 1000000; i++ {
		source.Add(&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("pod-%d", i)}})
	}
	factory := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 1*time.Second)

	informer := factory.Apps().V1().Deployments().Informer()
	handler := cache.ResourceEventHandlerFuncs{
		// do nothing
	}
	informer.AddEventHandler(handler)

	// Call informer.Run() twice as they are shared by the same client
	t.Run("factory first", func(t *testing.T) {
		stop := make(chan struct{})
		// i.e. indirectly through externalMetrics: apiCl.InformerFactory.Start(ctx.Done())
		factory.Start(stop)
		// i.e. orchestrator check init code
		go informer.Run(stop)
	})

	t.Run("factory second", func(t *testing.T) {
		stop := make(chan struct{})
		// i.e. orchestrator check init code
		go informer.Run(stop)
		// i.e. indirectly through externalMetrics: apiCl.InformerFactory.Start(ctx.Done())
		factory.Start(stop)
	})

	// wait a bit until we panic
	time.Sleep(30 * time.Second)
}

func TestInformerRunTwiceWithClose(t *testing.T) {
	source := fcache.NewFakeControllerSource()
	for i := 0; i < 1000000; i++ {
		source.Add(&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("pod-%d", i)}})
	}

	informer := cache.NewSharedInformer(source, &v1.Pod{}, 1*time.Second)
	handler := cache.ResourceEventHandlerFuncs{
		// do nothing
	}
	informer.AddEventHandler(handler)

	stop := make(chan struct{})

	// Call informer.Run() twice as they are shared by the same client

	// i.e. orchestrator check init code
	go informer.Run(stop)
	close(stop)
	time.Sleep(1 * time.Second)
	// i.e. indirectly through externalMetrics: apiCl.InformerFactory.Start(ctx.Done())
	go informer.Run(stop)

	// wait a bit until we panic
	time.Sleep(30 * time.Second)
}

func TestAssumption(t *testing.T) {
	// is run concurrent or blocking?
	factory := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 1*time.Second)
	stop := make(chan struct{})
	deploymentsInformer := factory.Apps().V1().Deployments().Informer()
	factory.Start(stop)
	cache.WaitForCacheSync(stop, deploymentsInformer.HasSynced)
	close(stop)
	time.Sleep(1 * time.Second)
	stop = make(chan struct{})
	factory.Start(stop)
	cache.WaitForCacheSync(stop, deploymentsInformer.HasSynced)

}
