// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package forwarder

import (
	"expvar"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	transactionsExpvars              = expvar.Map{}
	transactionsInputBytesByEndpoint = expvar.Map{}
	transactionsInputCountByEndpoint = expvar.Map{}
	transactionsDroppedOnInput       = expvar.Int{}
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

func initExpvars() {
	transactionsInputBytesByEndpoint.Init()
	transactionsInputCountByEndpoint.Init()
	transactionsRequeuedByEndpoint.Init()
	transactionsRetriedByEndpoint.Init()
	transactionsExpvars.Set("InputCountByEndpoint", &transactionsInputCountByEndpoint)
	transactionsExpvars.Set("InputBytesByEndpoint", &transactionsInputBytesByEndpoint)
	transactionsExpvars.Set("DroppedOnInput", &transactionsDroppedOnInput)
	transactionsExpvars.Set("Requeued", &transactionsRequeued)
	transactionsExpvars.Set("RequeuedByEndpoint", &transactionsRequeuedByEndpoint)
	transactionsExpvars.Set("Retried", &transactionsRetried)
	transactionsExpvars.Set("RetriedByEndpoint", &transactionsRetriedByEndpoint)
	transactionsExpvars.Set("RetryQueueSize", &transactionsRetryQueueSize)
}
