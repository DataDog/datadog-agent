// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tcp manages creation of tcp-based senders
package tcp

import (
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/tcp"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewTCPSender returns a new tcp sender.
func NewTCPSender(
	config pkgconfigmodel.Reader,
	sink sender.Sink,
	bufferSize int,
	serverlessMeta sender.ServerlessMeta,
	endpoints *config.Endpoints,
	destinationsCtx *client.DestinationsContext,
	status statusinterface.Status,
	componentName string,
	queueCount int,
	workersPerQueue int,
) *sender.Sender {
	log.Debugf("Creating a new sender for component %s with %d queues, %d tcp workers", componentName, queueCount, workersPerQueue)
	pipelineMonitor := metrics.NewTelemetryPipelineMonitor()

	destinationFactory := tcpDestinationFactory(endpoints, destinationsCtx, serverlessMeta, status)

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

func tcpDestinationFactory(
	endpoints *config.Endpoints,
	destinationsContext *client.DestinationsContext,
	serverlessMeta sender.ServerlessMeta,
	status statusinterface.Status,
) sender.DestinationFactory {
	isServerless := serverlessMeta != nil
	return func(_ string) *client.Destinations {
		reliable := []client.Destination{}
		additionals := []client.Destination{}
		for _, endpoint := range endpoints.GetReliableEndpoints() {
			reliable = append(reliable, tcp.NewDestination(endpoint, endpoints.UseProto, destinationsContext, !isServerless, status))
		}
		for _, endpoint := range endpoints.GetUnReliableEndpoints() {
			additionals = append(additionals, tcp.NewDestination(endpoint, endpoints.UseProto, destinationsContext, false, status))
		}

		return client.NewDestinations(reliable, additionals)
	}
}
