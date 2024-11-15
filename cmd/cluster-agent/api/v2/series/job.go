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
	loadstore "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/loadstore"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/time/rate"
	"k8s.io/client-go/util/workqueue"
)

const (
	subsystem               = "autoscaling_workload"
	payloadProcessQPS       = 500
	payloadProcessRateBurst = 50
)

var (
	commonOpts = telemetry.Options{NoDoubleUnderscoreSep: true}

	// telemetryWorkloadStoreMemory tracks the total memory usage of the store
	telemetryWorkloadStoreMemory = telemetry.NewGaugeWithOpts(
		subsystem,
		"store_memory_usage",
		nil,
		"Total memory usage of the store",
		commonOpts,
	)
	telemetryWorkloadMetricEntities = telemetry.NewGaugeWithOpts(
		subsystem,
		"store_metric_entities",
		[]string{"metric"},
		"Number of entities by metric names in the store",
		commonOpts,
	)
	telemetryWorkloadNamespaceEntities = telemetry.NewGaugeWithOpts(
		subsystem,
		"store_namespace_entities",
		[]string{"namespace"},
		"Number of entities by namespaces in the store",
		commonOpts,
	)
	telemetryWorkloadJobQueueLength = telemetry.NewCounterWithOpts(
		subsystem,
		"store_job_queue_length",
		[]string{"status"},
		"Length of the job queue",
		commonOpts,
	)
)

// jobQueue is a wrapper around workqueue.DelayingInterface to make it thread-safe.
type jobQueue struct {
	taskQueue workqueue.TypedRateLimitingInterface[*gogen.MetricPayload]
	isStarted bool
	store     loadstore.Store
	m         sync.Mutex
}

// newJobQueue creates a new jobQueue with  no delay for adding items
func newJobQueue(ctx context.Context) *jobQueue {
	q := jobQueue{
		taskQueue: workqueue.NewTypedRateLimitingQueue(workqueue.NewTypedMaxOfRateLimiter(
			&workqueue.TypedBucketRateLimiter[*gogen.MetricPayload]{
				Limiter: rate.NewLimiter(rate.Limit(payloadProcessQPS), payloadProcessRateBurst),
			},
		)),
		store:     loadstore.NewEntityStore(ctx),
		isStarted: false,
	}
	go q.start(ctx)
	return &q
}

func (jq *jobQueue) start(ctx context.Context) {
	jq.m.Lock()
	if jq.isStarted {
		return
	}
	jq.isStarted = true
	jq.m.Unlock()
	defer jq.taskQueue.ShutDown()
	jq.reportTelemetry(ctx)
	for {
		select {
		case <-ctx.Done():
			log.Infof("Stopping series payload job queue")
			return
		default:
			jq.processNextWorkItem()
		}
	}
}

func (jq *jobQueue) processNextWorkItem() bool {
	metricPayload, shutdown := jq.taskQueue.Get()
	if shutdown {
		return false
	}
	defer jq.taskQueue.Done(metricPayload)
	telemetryWorkloadJobQueueLength.Inc("processed")
	loadstore.ProcessLoadPayload(metricPayload, jq.store)
	return true
}

func (jq *jobQueue) addJob(payload *gogen.MetricPayload) {
	jq.taskQueue.Add(payload)
	telemetryWorkloadJobQueueLength.Inc("queued")
}

func (jq *jobQueue) reportTelemetry(ctx context.Context) {
	go func() {
		infoTicker := time.NewTicker(60 * time.Second)
		for {
			select {
			case <-ctx.Done():
				return
			case <-infoTicker.C:
				info := jq.store.GetStoreInfo()
				telemetryWorkloadStoreMemory.Set(float64(info.TotalMemoryUsage))
				for k, v := range info.EntityCountByMetric {
					telemetryWorkloadMetricEntities.Set(float64(v), k)
				}
				for k, v := range info.EntityCountByNamespace {
					telemetryWorkloadNamespaceEntities.Set(float64(v), k)
				}
				log.Debugf("Store info: %+v", info)
			}
		}
	}()
}
