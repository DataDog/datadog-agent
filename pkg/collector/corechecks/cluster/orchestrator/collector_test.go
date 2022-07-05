/*
 * Unless explicitly stated otherwise all files in this repository are licensed
 * under the Apache License Version 2.0.
 * This product includes software developed at Datadog (https://www.datadoghq.com/).
 * Copyright 2016-2022 Datadog, Inc.
 */

package orchestrator

import (
	"fmt"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/tools/cache"
	fcache "k8s.io/client-go/tools/cache/testing"
)

func TestInformerRunTwice(t *testing.T) {
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
	defer close(stop)

	// Call informer.Run() twice as they are shared by the same client

	// i.e. orchestrator check init code
	go informer.Run(stop)
	// i.e. indirectly through externalMetrics: apiCl.InformerFactory.Start(ctx.Done())
	go informer.Run(stop)

	// wait a bit until we panic
	time.Sleep(30 * time.Second)
}
