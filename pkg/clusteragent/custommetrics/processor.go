// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package custommetrics

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	metricsChannelSize = 1024
)

// PodMetricValue is a metric value from a pod submitted to the intake.
type PodMetricValue struct {
	MetricName string
	PodName    string
	Namespace  string
	Timestamp  float64
	Value      float64
}

// Processor is an interface for processing pod metrics from the intake.
type Processor interface {
	Start()
	Process(PodMetricValue)
	Stop() error
}

// BufferedProcessor buffers pod metrics for the duration specified by the flush interval
// before computing object/pod metrics and persisting them to persistent storage.
type BufferedProcessor struct {
	flushInterval time.Duration
	metricsCh     chan PodMetricValue
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

// NewBufferedProcessor returns a new processor that can be passed to the intake.
func NewBufferedProcessor() (*BufferedProcessor, error) {
	return &BufferedProcessor{
		flushInterval: config.Datadog.GetDuration("cluster_agent.custom_metrics.processor_flush_interval") * time.Second,
		metricsCh:     make(chan PodMetricValue, metricsChannelSize),
		stopCh:        make(chan struct{}),
	}, nil
}

// Start ...
func (p *BufferedProcessor) Start() {
	log.Info("Starting Buffered Processor...")
	p.wg.Add(1)
	go p.start()
}

func (p *BufferedProcessor) start() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.flushInterval)

	bundle := make([]PodMetricValue, 0)
	for {
		select {
		case <-p.stopCh:
			return
		case pod := <-p.metricsCh:
			bundle = append(bundle, pod)
		case <-ticker.C:
			p.flush(bundle)
			bundle = make([]PodMetricValue, 0)
		}
	}
}

func (p *BufferedProcessor) flush(bundle []PodMetricValue) {
	log.Tracef("Flushed bundle of %d pod metrics", len(bundle))
	// Compute average of each pods metric
	// aggregated := make(map[string]PodsMetricValue)
	// for metricName, pods := range bundle {
	// 	podSum := float64(0)
	// 	ts := float64(0)
	// 	for _, pod := range pods {
	// 		podSum += pod.Value
	// 		if pod.Timestamp > ts {
	// 			ts = pod.Timestamp
	// 		}
	// 	}
	// 	// TODO: store in configmap
	// }
	// data, err := p.store.Data()
	// if err != nil {
	// 	// oh noes!
	// }
	// for _, pm := range bundle {
	// 	cm, ok := data.Get(pm.Namespace, pm.MetricName)
	// 	if !ok {
	// 		continue
	// 	}
	// 	cm.Timestamp = pm.Timestamp
	// 	cm.Value = pm.Value
	// 	data.Set(cm)
	// }
	// if err = p.store.Update(); err != nil {
	// 	// oh noes!
	// }
}

// Process submits a pod metric from the intake to be processed.
func (p *BufferedProcessor) Process(podMetric PodMetricValue) {
	p.metricsCh <- podMetric
}

// Stop gracefully stops the processor.
func (p *BufferedProcessor) Stop() error {
	close(p.stopCh)
	p.wg.Wait()
	return nil
}
