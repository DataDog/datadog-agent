// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarder

import (
	"expvar"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	pkgorchestratormodel "github.com/DataDog/datadog-agent/pkg/orchestrator/model"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	transactionsIntakeOrchestrator = map[pkgorchestratormodel.NodeType]*expvar.Int{}

	highPriorityQueueFull            = expvar.Int{}
	transactionsInputBytesByEndpoint = expvar.Map{}
	transactionsInputCountByEndpoint = expvar.Map{}
	transactionsRequeued             = expvar.Int{}
	transactionsRequeuedByEndpoint   = expvar.Map{}
	transactionsRetried              = expvar.Int{}
	transactionsRetriedByEndpoint    = expvar.Map{}
	transactionsRetryQueueSize       = expvar.Int{}
	transactionsOrchestratorManifest = expvar.Int{}

	tlmTxInputBytes = telemetry.NewCounter("transactions", "input_bytes",
		[]string{"domain", "endpoint"}, "Incoming transaction sizes in bytes")
	tlmTxInputCount = telemetry.NewCounter("transactions", "input_count",
		[]string{"domain", "endpoint"}, "Incoming transaction count")
	tlmTxHighPriorityQueueFull = telemetry.NewCounter("transactions", "high_priority_queue_full",
		[]string{"domain", "endpoint"}, "Count of transactions added to the retry queue because the high priority queue is full")
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
		endpoints.ConnectionsEndpoint,
		endpoints.ContainerEndpoint,
		endpoints.EventsEndpoint,
		endpoints.HostMetadataEndpoint,
		endpoints.OrchestratorEndpoint,
		endpoints.ProcessesEndpoint,
		endpoints.RtContainerEndpoint,
		endpoints.RtProcessesEndpoint,
		endpoints.SeriesEndpoint,
		endpoints.ServiceChecksEndpoint,
		endpoints.SketchSeriesEndpoint,
		endpoints.V1CheckRunsEndpoint,
		endpoints.V1IntakeEndpoint,
		endpoints.V1SeriesEndpoint,
		endpoints.V1SketchSeriesEndpoint,
		endpoints.V1ValidateEndpoint,
	}

	for _, endpoint := range endpoints {
		transaction.TransactionsSuccessByEndpoint.Set(endpoint.Name, expvar.NewInt(endpoint.Name))
	}
}

func initOrchestratorExpVars() {
	for _, nodeType := range pkgorchestratormodel.NodeTypes() {
		transactionsIntakeOrchestrator[nodeType] = &expvar.Int{}
		transaction.TransactionsExpvars.Set(nodeType.String(), transactionsIntakeOrchestrator[nodeType])
	}
	transaction.TransactionsExpvars.Set("OrchestratorManifest", &transactionsOrchestratorManifest)
}

func bumpOrchestratorPayload(log log.Component, nodeType int) {
	e, ok := transactionsIntakeOrchestrator[pkgorchestratormodel.NodeType(nodeType)]
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
	transaction.TransactionsExpvars.Set("HighPriorityQueueFull", &highPriorityQueueFull)
	transaction.TransactionsExpvars.Set("Requeued", &transactionsRequeued)
	transaction.TransactionsExpvars.Set("RequeuedByEndpoint", &transactionsRequeuedByEndpoint)
	transaction.TransactionsExpvars.Set("Retried", &transactionsRetried)
	transaction.TransactionsExpvars.Set("RetriedByEndpoint", &transactionsRetriedByEndpoint)
	transaction.TransactionsExpvars.Set("RetryQueueSize", &transactionsRetryQueueSize)
}
