// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package forwarder

import (
	"expvar"

	"github.com/DataDog/datadog-agent/pkg/forwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	transactionsIntakeOrchestrator = map[orchestrator.NodeType]*expvar.Int{}

	v1SeriesEndpoint       = transaction.Endpoint{Route: "/api/v1/series", Name: "series_v1"}
	v1CheckRunsEndpoint    = transaction.Endpoint{Route: "/api/v1/check_run", Name: "check_run_v1"}
	v1IntakeEndpoint       = transaction.Endpoint{Route: "/intake/", Name: "intake"}
	v1SketchSeriesEndpoint = transaction.Endpoint{Route: "/api/v1/sketches", Name: "sketches_v1"} // nolint unused for now
	v1ValidateEndpoint     = transaction.Endpoint{Route: "/api/v1/validate", Name: "validate_v1"}

	seriesEndpoint        = transaction.Endpoint{Route: "/api/v2/series", Name: "series_v2"}
	eventsEndpoint        = transaction.Endpoint{Route: "/api/v2/events", Name: "events_v2"}
	serviceChecksEndpoint = transaction.Endpoint{Route: "/api/v2/service_checks", Name: "services_checks_v2"}
	sketchSeriesEndpoint  = transaction.Endpoint{Route: "/api/beta/sketches", Name: "sketches_v2"}
	hostMetadataEndpoint  = transaction.Endpoint{Route: "/api/v2/host_metadata", Name: "host_metadata_v2"}
	metadataEndpoint      = transaction.Endpoint{Route: "/api/v2/metadata", Name: "metadata_v2"}

	processesEndpoint        = transaction.Endpoint{Route: "/api/v1/collector", Name: "process"}
	processDiscoveryEndpoint = transaction.Endpoint{Route: "/api/v1/collector", Name: "processDiscovery"}
	rtProcessesEndpoint      = transaction.Endpoint{Route: "/api/v1/collector", Name: "rtprocess"}
	containerEndpoint        = transaction.Endpoint{Route: "/api/v1/container", Name: "container"}
	rtContainerEndpoint      = transaction.Endpoint{Route: "/api/v1/container", Name: "rtcontainer"}
	connectionsEndpoint      = transaction.Endpoint{Route: "/api/v1/collector", Name: "connections"}
	orchestratorEndpoint     = transaction.Endpoint{Route: "/api/v1/orchestrator", Name: "orchestrator"}

	transactionsDroppedOnInput       = expvar.Int{}
	transactionsInputBytesByEndpoint = expvar.Map{}
	transactionsInputCountByEndpoint = expvar.Map{}
	transactionsRequeued             = expvar.Int{}
	transactionsRequeuedByEndpoint   = expvar.Map{}
	transactionsRetried              = expvar.Int{}
	transactionsRetriedByEndpoint    = expvar.Map{}
	transactionsRetryQueueSize       = expvar.Int{}

	tlmTxInputBytes = telemetry.NewCounter("transactions", "input_bytes",
		[]string{"domain", "endpoint"}, "Incoming transaction sizes in bytes")
	tlmTxInputCount = telemetry.NewCounter("transactions", "input_count",
		[]string{"domain", "endpoint"}, "Incoming transaction count")
	tlmTxDroppedOnInput = telemetry.NewCounter("transactions", "dropped_on_input",
		[]string{"domain", "endpoint"}, "Count of transactions dropped on input")
	tlmTxRequeued = telemetry.NewCounter("transactions", "requeued",
		[]string{"domain", "endpoint"}, "Transaction requeue count")
	tlmTxRetried = telemetry.NewCounter("transactions", "retries",
		[]string{"domain", "endpoint"}, "Transaction retry count")
	tlmTxRetryQueueSize = telemetry.NewGauge("transactions", "retry_queue_size",
		[]string{"domain"}, "Retry queue size")
)

func init() {
	initOrchestratorExpVars()
	initTransactionsExpvars()
	initForwarderHealthExpvars()
	initEndpointExpvars()
}

func initEndpointExpvars() {
	endpoints := []transaction.Endpoint{
		connectionsEndpoint,
		containerEndpoint,
		eventsEndpoint,
		hostMetadataEndpoint,
		metadataEndpoint,
		orchestratorEndpoint,
		processesEndpoint,
		rtContainerEndpoint,
		rtProcessesEndpoint,
		seriesEndpoint,
		serviceChecksEndpoint,
		sketchSeriesEndpoint,
		v1CheckRunsEndpoint,
		v1IntakeEndpoint,
		v1SeriesEndpoint,
		v1SketchSeriesEndpoint,
		v1ValidateEndpoint,
	}

	for _, endpoint := range endpoints {
		transaction.TransactionsSuccessByEndpoint.Set(endpoint.Name, expvar.NewInt(endpoint.Name))
	}
}

func initOrchestratorExpVars() {
	for _, nodeType := range orchestrator.NodeTypes() {
		transactionsIntakeOrchestrator[nodeType] = &expvar.Int{}
		transaction.TransactionsExpvars.Set(nodeType.String(), transactionsIntakeOrchestrator[nodeType])
	}
}

func bumpOrchestratorPayload(nodeType int) {
	e, ok := transactionsIntakeOrchestrator[orchestrator.NodeType(nodeType)]
	if !ok {
		log.Errorf("Unknown NodeType %v, cannot bump expvar", nodeType)
		return
	}
	e.Add(1)
}

func initTransactionsExpvars() {
	transactionsInputBytesByEndpoint.Init()
	transactionsInputCountByEndpoint.Init()
	transactionsRequeuedByEndpoint.Init()
	transactionsRetriedByEndpoint.Init()
	transaction.TransactionsExpvars.Set("InputCountByEndpoint", &transactionsInputCountByEndpoint)
	transaction.TransactionsExpvars.Set("InputBytesByEndpoint", &transactionsInputBytesByEndpoint)
	transaction.TransactionsExpvars.Set("DroppedOnInput", &transactionsDroppedOnInput)
	transaction.TransactionsExpvars.Set("Requeued", &transactionsRequeued)
	transaction.TransactionsExpvars.Set("RequeuedByEndpoint", &transactionsRequeuedByEndpoint)
	transaction.TransactionsExpvars.Set("Retried", &transactionsRetried)
	transaction.TransactionsExpvars.Set("RetriedByEndpoint", &transactionsRetriedByEndpoint)
	transaction.TransactionsExpvars.Set("RetryQueueSize", &transactionsRetryQueueSize)
}
