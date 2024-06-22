// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package postgres

import (
	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Telemetry is a struct to hold the telemetry for the kafka protocol
type Telemetry struct {
	metricGroup *libtelemetry.MetricGroup

	exceededQueryLength       *libtelemetry.Counter // this happens when the query length reaches capacity
	failedTableNameExtraction *libtelemetry.Counter // this happens when we failed to extract the table name
	failedOperationExtraction *libtelemetry.Counter // this happens when we failed to extract the operation
}

// NewTelemetry creates a new Telemetry
func NewTelemetry() *Telemetry {
	metricGroup := libtelemetry.NewMetricGroup("usm.postgres")

	return &Telemetry{
		metricGroup:               metricGroup,
		exceededQueryLength:       metricGroup.NewCounter("exceeded_query_length", libtelemetry.OptStatsd),
		failedTableNameExtraction: metricGroup.NewCounter("failed_table_name_extraction", libtelemetry.OptStatsd),
		failedOperationExtraction: metricGroup.NewCounter("failed_operation_extraction", libtelemetry.OptStatsd),
	}
}

// Count increments the telemetry counters
func (t *Telemetry) Count(tx *EbpfTx, eventWrapper *EventWrapper) {
	if tx.Original_query_size > BufferSize {
		t.exceededQueryLength.Add(1)
	}
	if eventWrapper.operation == UnknownOP {
		t.failedOperationExtraction.Add(1)
	}
	if eventWrapper.tableName == "UNKNOWN" {
		t.failedTableNameExtraction.Add(1)
	}

}

// Log logs the postgres stats summary
func (t *Telemetry) Log() {
	log.Debugf("postgres stats summary: %s", t.metricGroup.Summary())
}
