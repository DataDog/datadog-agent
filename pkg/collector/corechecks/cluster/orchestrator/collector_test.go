/*
 * Unless explicitly stated otherwise all files in this repository are licensed
 * under the Apache License Version 2.0.
 * This product includes software developed at Datadog (https://www.datadoghq.com/).
 * Copyright 2016-2022 Datadog, Inc.
 */

package orchestrator

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	fcache "k8s.io/client-go/tools/cache/testing"
)

// TestFactoryNotRestartedAfterClose close simulates the check being closed and then restarted with .Start again
func TestFactoryNotRestartedAfterClose(t *testing.T) {
	client := fake.NewSimpleClientset()

	writeFirstNS(client)

	factory := informers.NewSharedInformerFactory(client, 1*time.Second)
	stop := make(chan struct{})
	informer := factory.Core().V1().Namespaces().Informer()
	// empty
	keys := informer.GetStore().ListKeys()
	assert.Equal(t, keys, []string{})
	factory.Start(stop)
	time.Sleep(1 * time.Second)
	cache.WaitForCacheSync(stop, informer.HasSynced)

	// not empty anymore as we started the informer sync
	keys = informer.GetStore().ListKeys()
	assert.Equal(t, keys, []string{"kube-system"})

	// stopping as simulating a reschedule
	close(stop)
	writeSecondNS(client)

	// we get to the same worker again
	time.Sleep(1 * time.Second)
	keys = informer.GetStore().ListKeys()
	// expect only one key as informer sync stopped before the second got written
	assert.Equal(t, keys, []string{"kube-system"})

	// we get to the same worker again, therefore let's start the informer again!
	stop2 := make(chan struct{})
	factory.Start(stop2)
	time.Sleep(1 * time.Second)
	cache.WaitForCacheSync(stop2, informer.HasSynced)
	keys = informer.GetStore().ListKeys()
	// expect keys the second key to show as we restarted the informer, but because we use Start() we know that we didn't resync, therefore its one
	assert.Len(t, keys, 1)

	//informer.Run(stop2)
	//cache.WaitForCacheSync(stop2, informer.HasSynced)
	//// expect keys to have synced because we manually restarted and not through the factory
	//assert.Len(t, keys, 2)
}

func writeFirstNS(client *fake.Clientset) {
	kubeNs := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "123",
			UID:             "226430c6-5e57-11ea-91d5-42010a8400c6",
			Name:            "kube-system",
		},
	}
	_, _ = client.CoreV1().Namespaces().Create(context.TODO(), &kubeNs, metav1.CreateOptions{})
}

func writeSecondNS(client *fake.Clientset) {
	kubeNs2 := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "423",
			UID:             "126430c6-5e57-11ea-91d5-42010a8400c6",
			Name:            "another-system",
		},
	}
	_, _ = client.CoreV1().Namespaces().Create(context.TODO(), &kubeNs2, metav1.CreateOptions{})
}

func TestInformerRunTwiceWillFailWithFactory(t *testing.T) {
	factory := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 1*time.Second)

	informer := factory.Apps().V1().Deployments().Informer()
	handler := cache.ResourceEventHandlerFuncs{
		// do nothing
	}
	informer.AddEventHandler(handler)

	// Call informer.Run() twice as they are shared by the same client
	t.Run("factory first - race", func(t *testing.T) {
		stop := make(chan struct{})
		// i.e. indirectly through externalMetrics: apiCl.InformerFactory.Start(ctx.Done())
		factory.Start(stop)
		// i.e. orchestrator check init code
		go informer.Run(stop)
		time.Sleep(10 * time.Second)
	})

	t.Run("factory second - race", func(t *testing.T) {
		stop := make(chan struct{})
		// i.e. orchestrator check init code
		go informer.Run(stop)
		// i.e. indirectly through externalMetrics: apiCl.InformerFactory.Start(ctx.Done())
		factory.Start(stop)
		time.Sleep(10 * time.Second)
	})

	// -> The sharedIndexInformer has started, run more than once is not allowed <-- logs from our agent
	t.Run("informer twice - race", func(t *testing.T) {
		stop := make(chan struct{})
		// i.e. orchestrator check init code
		go informer.Run(stop)
		go informer.Run(stop)
		time.Sleep(10 * time.Second)
	})
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
