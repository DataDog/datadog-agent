// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package http manages creation of http-based senders
package http

import (
	"strconv"
	"sync"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewHTTPSender returns a new http sender.
func NewHTTPSender(
	config pkgconfigmodel.Reader,
	auditor auditor.Auditor,
	bufferSize int,
	senderDoneChan chan *sync.WaitGroup,
	flushWg *sync.WaitGroup,
	endpoints *config.Endpoints,
	destinationsCtx *client.DestinationsContext,
	serverless bool,
	componentName string,
	contentType string,
	queueCount int,
	workersPerQueue int,
	minWorkerConcurrency int,
	maxWorkerConcurrency int,
) *sender.Sender {
	log.Debugf(
		"Creating a new sender for component %s with %d queues, %d http workers, %d min sender concurrency, and %d max sender concurrency",
		componentName,
		queueCount,
		workersPerQueue,
		minWorkerConcurrency,
		maxWorkerConcurrency,
	)
	pipelineMonitor := metrics.NewTelemetryPipelineMonitor("http_sender")

	destinationFactory := httpDestinationFactory(
		endpoints,
		destinationsCtx,
		pipelineMonitor,
		serverless,
		senderDoneChan,
		config,
		componentName,
		contentType,
		minWorkerConcurrency,
		maxWorkerConcurrency,
	)

	return sender.NewSenderV2(
		config,
		auditor,
		destinationFactory,
		bufferSize,
		senderDoneChan,
		flushWg,
		queueCount,
		workersPerQueue,
		pipelineMonitor,
	)
}

func httpDestinationFactory(
	endpoints *config.Endpoints,
	destinationsContext *client.DestinationsContext,
	pipelineMonitor metrics.PipelineMonitor,
	serverless bool,
	senderDoneChan chan *sync.WaitGroup,
	cfg pkgconfigmodel.Reader,
	componentName string,
	contentyType string,
	minConcurrency int,
	maxConcurrency int,
) sender.DestinationFactory {
	return func() *client.Destinations {
		reliable := []client.Destination{}
		additionals := []client.Destination{}
		for i, endpoint := range endpoints.GetReliableEndpoints() {
			destMeta := client.NewDestinationMetadata(componentName, pipelineMonitor.ID(), "reliable", strconv.Itoa(i))
			if serverless {
				reliable = append(reliable, http.NewSyncDestination(endpoint, contentyType, destinationsContext, senderDoneChan, destMeta, cfg))
			} else {
				reliable = append(reliable, http.NewDestination(endpoint, contentyType, destinationsContext, true, destMeta, cfg, minConcurrency, maxConcurrency, pipelineMonitor))
			}
		}
		for i, endpoint := range endpoints.GetUnReliableEndpoints() {
			destMeta := client.NewDestinationMetadata(componentName, pipelineMonitor.ID(), "unreliable", strconv.Itoa(i))
			if serverless {
				additionals = append(additionals, http.NewSyncDestination(endpoint, contentyType, destinationsContext, senderDoneChan, destMeta, cfg))
			} else {
				additionals = append(additionals, http.NewDestination(endpoint, contentyType, destinationsContext, false, destMeta, cfg, minConcurrency, maxConcurrency, pipelineMonitor))
			}
		}
		return client.NewDestinations(reliable, additionals)
	}
}
