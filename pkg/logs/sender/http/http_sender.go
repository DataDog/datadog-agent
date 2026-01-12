// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package http manages creation of http-based senders
package http

import (
	"strconv"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewHTTPSender returns a new http sender.
func NewHTTPSender(
	config pkgconfigmodel.Reader,
	sink sender.Sink,
	bufferSize int,
	serverlessMeta sender.ServerlessMeta,
	endpoints *config.Endpoints,
	destinationsCtx *client.DestinationsContext,
	componentName string,
	contentType string,
	evpCategory string,
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
	pipelineMonitor := metrics.NewTelemetryPipelineMonitor()

	destinationFactory := httpDestinationFactory(
		endpoints,
		destinationsCtx,
		pipelineMonitor,
		serverlessMeta,
		config,
		componentName,
		contentType,
		evpCategory,
		minWorkerConcurrency,
		maxWorkerConcurrency,
	)

	return sender.NewSender(
		config,
		sink,
		destinationFactory,
		bufferSize,
		serverlessMeta,
		queueCount,
		workersPerQueue,
		pipelineMonitor,
	)
}

func httpDestinationFactory(
	endpoints *config.Endpoints,
	destinationsContext *client.DestinationsContext,
	pipelineMonitor metrics.PipelineMonitor,
	serverlessMeta sender.ServerlessMeta,
	cfg pkgconfigmodel.Reader,
	componentName string,
	contentyType string,
	evpCategory string,
	minConcurrency int,
	maxConcurrency int,
) sender.DestinationFactory {
	return func(instanceID string) *client.Destinations {
		reliable := []client.Destination{}
		additionals := []client.Destination{}
		for i, endpoint := range endpoints.GetReliableEndpoints() {
			destMeta := client.NewDestinationMetadata(componentName, instanceID, "reliable", strconv.Itoa(i), evpCategory)
			if serverlessMeta.IsEnabled() {
				reliable = append(reliable, http.NewSyncDestination(endpoint, contentyType, destinationsContext, serverlessMeta.SenderDoneChan(), destMeta, cfg))
			} else {
				reliable = append(reliable, http.NewDestination(endpoint, contentyType, destinationsContext, true, destMeta, cfg, minConcurrency, maxConcurrency, pipelineMonitor, instanceID))
			}
		}
		for i, endpoint := range endpoints.GetUnReliableEndpoints() {
			destMeta := client.NewDestinationMetadata(componentName, instanceID, "unreliable", strconv.Itoa(i), evpCategory)
			if serverlessMeta.IsEnabled() {
				additionals = append(additionals, http.NewSyncDestination(endpoint, contentyType, destinationsContext, serverlessMeta.SenderDoneChan(), destMeta, cfg))
			} else {
				additionals = append(additionals, http.NewDestination(endpoint, contentyType, destinationsContext, false, destMeta, cfg, minConcurrency, maxConcurrency, pipelineMonitor, instanceID))
			}
		}
		return client.NewDestinations(reliable, additionals)
	}
}
