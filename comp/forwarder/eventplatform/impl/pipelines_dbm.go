// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventplatformimpl

import (
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

const (
	eventTypeDBMSamples          = "dbm-samples"
	eventTypeDBMMetrics          = "dbm-metrics"
	eventTypeDBMActivity         = "dbm-activity"
	eventTypeDBMMetadata         = "dbm-metadata"
	eventTypeDBMHealth           = "dbm-health"
	eventTypeDBMColumnStatistics = "dbm-column-statistics"
)

func getDBMPipelines() []passthroughPipelineDesc {
	return []passthroughPipelineDesc{
		{
			eventType:              eventTypeDBMSamples,
			category:               "DBM",
			contentType:            logshttp.JSONContentType,
			endpointsConfigPrefix:  "database_monitoring.samples.",
			hostnameEndpointPrefix: "dbm-metrics-intake.",
			intakeTrackType:        "databasequery",
			// raise the default batch_max_concurrent_send from 0 to 10 to ensure this pipeline is able to handle 4k events/s
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    10e6,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			// High input chan size is needed to handle high number of DBM events being flushed by DBM integrations
			defaultInputChanSize: 500,
		},
		{
			eventType:              eventTypeDBMMetrics,
			category:               "DBM",
			contentType:            logshttp.JSONContentType,
			endpointsConfigPrefix:  "database_monitoring.metrics.",
			hostnameEndpointPrefix: "dbm-metrics-intake.",
			intakeTrackType:        "dbmmetrics",
			// raise the default batch_max_concurrent_send from 0 to 10 to ensure this pipeline is able to handle 4k events/s
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    20e6,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			// High input chan size is needed to handle high number of DBM events being flushed by DBM integrations
			defaultInputChanSize: 500,
		},
		{
			eventType:   eventTypeDBMMetadata,
			contentType: logshttp.JSONContentType,
			// set the endpoint config to "metrics" since metadata will hit the same endpoint
			// as metrics, so there is no need to add an extra config endpoint.
			// As a follow-on PR, we should clean this up to have a single config for each track type since
			// all of our data now flows through the same intake
			endpointsConfigPrefix:  "database_monitoring.metrics.",
			hostnameEndpointPrefix: "dbm-metrics-intake.",
			intakeTrackType:        "dbmmetadata",
			// raise the default batch_max_concurrent_send from 0 to 10 to ensure this pipeline is able to handle 4k events/s
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    20e6,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			// High input chan size is needed to handle high number of DBM events being flushed by DBM integrations
			defaultInputChanSize: 500,
		},
		{
			eventType:              eventTypeDBMActivity,
			category:               "DBM",
			contentType:            logshttp.JSONContentType,
			endpointsConfigPrefix:  "database_monitoring.activity.",
			hostnameEndpointPrefix: "dbm-metrics-intake.",
			intakeTrackType:        "dbmactivity",
			// raise the default batch_max_concurrent_send from 0 to 10 to ensure this pipeline is able to handle 4k events/s
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    20e6,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			// High input chan size is needed to handle high number of DBM events being flushed by DBM integrations
			defaultInputChanSize: 500,
		},
		{
			eventType:   eventTypeDBMHealth,
			contentType: logshttp.JSONContentType,
			// set the endpoint config to "metrics" since health will hit the same endpoint
			// as metrics, so there is no need to add an extra config endpoint.
			endpointsConfigPrefix:  "database_monitoring.metrics.",
			hostnameEndpointPrefix: "dbm-metrics-intake.",
			intakeTrackType:        "dbmhealth",
			// raise the default batch_max_concurrent_send from 0 to 10 to ensure this pipeline is able to handle 4k events/s
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    20e6,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			// High input chan size is needed to handle high number of DBM events being flushed by DBM integrations
			defaultInputChanSize: 500,
		},
		{
			eventType:   eventTypeDBMColumnStatistics,
			contentType: logshttp.JSONContentType,
			// set the endpoint config to "metrics" since column statistics will hit the same endpoint
			// as metrics, so there is no need to add an extra config endpoint.
			endpointsConfigPrefix:  "database_monitoring.metrics.",
			hostnameEndpointPrefix: "dbm-metrics-intake.",
			intakeTrackType:        "dbmcolumnstatistics",
			// raise the default batch_max_concurrent_send from 0 to 10 to ensure this pipeline is able to handle 4k events/s
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    20e6,
			defaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			// High input chan size is needed to handle high number of DBM events being flushed by DBM integrations
			defaultInputChanSize: 500,
		},
	}
}
