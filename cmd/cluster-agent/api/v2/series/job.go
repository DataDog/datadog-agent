// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package series

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/agent-payload/v5/gogen"
	loadstore "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/loadstore"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
)

// jobQueue is a wrapper around workqueue.DelayingInterface to make it thread-safe.
type jobQueue struct {
	queue     workqueue.DelayingInterface
	isStarted bool
	store     loadstore.Store
	m         sync.Mutex
}

// newJobQueue creates a new jobQueue with  no delay for adding items
func newJobQueue(ctx context.Context) *jobQueue {
	q := jobQueue{
		queue: workqueue.NewDelayingQueueWithConfig(workqueue.DelayingQueueConfig{
			Name: "seriesPayloadJobQueue",
		}),
		store:     loadstore.NewEntityStore(ctx),
		isStarted: false,
	}
	go q.start(ctx)
	return &q
}

func (jq *jobQueue) worker() {
	for jq.processNextWorkItem() {
	}
}

func (jq *jobQueue) start(ctx context.Context) {
	jq.m.Lock()
	if jq.isStarted {
		return
	}
	jq.isStarted = true
	jq.m.Unlock()
	defer jq.queue.ShutDown()
	go wait.Until(jq.worker, time.Second, ctx.Done())
	infoTicker := time.NewTicker(60 * time.Second)
	for {
		select {
		case <-ctx.Done():
			log.Infof("Stopping series payload job queue")
			return
		case <-infoTicker.C:
			log.Infof("Loadstore info: %s", jq.store.GetStoreInfo())
		}
	}
}

func (jq *jobQueue) processNextWorkItem() bool {
	obj, shutdown := jq.queue.Get()
	if shutdown {
		return false
	}
	defer jq.queue.Done(obj)
	metricPayload, ok := obj.(*gogen.MetricPayload)
	if !ok {
		log.Errorf("Expected MetricPayload but got %T", obj)
		return true
	}
	loadstore.ProcessLoadPayload(metricPayload, jq.store)
	return true
}

func (jq *jobQueue) addJob(payload *gogen.MetricPayload) {
	jq.queue.Add(payload)
}
