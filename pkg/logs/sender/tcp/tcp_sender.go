// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tcp manages creation of tcp-based senders
package tcp

import (
	"sync"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
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
	auditor auditor.Auditor,
	bufferSize int,
	senderDoneChan chan *sync.WaitGroup,
	flushWg *sync.WaitGroup,
	endpoints *config.Endpoints,
	destinationsCtx *client.DestinationsContext,
	status statusinterface.Status,
	serverless bool,
	componentName string,
	queueCount int,
	workersPerQueue int,
) *sender.Sender {
	log.Debugf("Creating a new sender for component %s with %d queues, %d tcp workers", componentName, queueCount, workersPerQueue)
	pipelineMonitor := metrics.NewTelemetryPipelineMonitor("tcp_sender")

	destinationFactory := tcpDestinationFactory(endpoints, destinationsCtx, serverless, status)

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

func tcpDestinationFactory(
	endpoints *config.Endpoints,
	destinationsContext *client.DestinationsContext,
	serverless bool,
	status statusinterface.Status,
) sender.DestinationFactory {
	return func() *client.Destinations {
		reliable := []client.Destination{}
		additionals := []client.Destination{}
		for _, endpoint := range endpoints.GetReliableEndpoints() {
			reliable = append(reliable, tcp.NewDestination(endpoint, endpoints.UseProto, destinationsContext, !serverless, status))
		}
		for _, endpoint := range endpoints.GetUnReliableEndpoints() {
			additionals = append(additionals, tcp.NewDestination(endpoint, endpoints.UseProto, destinationsContext, false, status))
		}

		return client.NewDestinations(reliable, additionals)
	}
}
