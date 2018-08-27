// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package custommetrics

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/tags"
)

const (
	seriesChannelSize = 1024
)

// Intake ...
type Intake struct {
	numGoroutines int
	processor     Processor
	seriesCh      chan metrics.Payload
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

// Intake ...
func NewIntake(processor Processor) (*Intake, error) {
	return &Intake{
		numGoroutines: config.Datadog.GetInt("cluster_agent.custom_metrics.intake_num_workers"),
		processor:     processor,
		seriesCh:      make(chan metrics.Payload, seriesChannelSize),
		stopCh:        make(chan struct{}),
	}, nil
}

// Start ...
func (in *Intake) Start() {
	log.Info("Starting Metrics Intake...")
	for i := 0; i < in.numGoroutines; i++ {
		in.wg.Add(1)
		go in.worker()
	}
	in.processor.Start()
}

func (in *Intake) worker() {
	defer in.wg.Done()

	for {
		select {
		case <-in.stopCh:
			return
		case payload := <-in.seriesCh:
			log.Tracef("Processing payload with %d series", len(payload.Series))

			for _, serie := range payload.Series {
				if len(serie.Points) == 0 {
					log.Tracef("Dropping serie: no points: %v", serie)
					continue
				}
				log.Tracef("Processing serie %v from %s", serie, serie.Host)

				// HACK(devonboyer): pod_name is a high cardinality tag
				var podName string
				if podName = tags.Get(serie.Tags, "pod_name"); podName == "" {
					log.Tracef("Dropping serie: missing tag \"pod_name\": %v", serie)
					continue
				}

				var ns string
				if ns = tags.Get(serie.Tags, "kube_namespace"); ns == "" {
					log.Tracef("Dropping serie: missing tag \"kube_namespace\": %v", serie)
					continue
				}

				in.processor.Process(
					PodMetricValue{
						PodName:    podName,
						Namespace:  ns,
						MetricName: serie.Name,
						Timestamp:  serie.Points[0].Ts,
						Value:      serie.Points[0].Value,
					})

			}
		}
	}
}

// Submit submits a payload to the intake to be processed.
func (in *Intake) Submit(payload metrics.Payload) error {
	select {
	case in.seriesCh <- payload:
	default:
		return fmt.Errorf("intake buffer is full, dropping payload")
	}
	return nil
}

// Stop gracefully stops the intake.
func (in *Intake) Stop() error {
	close(in.stopCh)
	in.wg.Wait()
	return in.processor.Stop()
}
