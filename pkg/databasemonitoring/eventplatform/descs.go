// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package eventplatform owns the event platform pipeline descriptions for
// database monitoring (DBM). Each PipelineDesc returned by Descs() is contributed
// to the event platform forwarder via the "ep_pipeline_descs" fx group, so the
// DBM team owns this configuration rather than the logs agent team.
package eventplatform

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// team: database-monitoring

// Descs returns the pipeline descriptions for all DBM event types.
func Descs() []eventplatform.PipelineDesc {
	return []eventplatform.PipelineDesc{
		{
			EventType:              "dbm-samples",
			Category:               "DBM",
			ContentType:            logshttp.JSONContentType,
			EndpointsConfigPrefix:  "database_monitoring.samples.",
			HostnameEndpointPrefix: "dbm-metrics-intake.",
			IntakeTrackType:        "databasequery",
			// raise the default batch_max_concurrent_send from 0 to 10 to ensure this pipeline is able to handle 4k events/s
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    10e6,
			DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			// High input chan size is needed to handle high number of DBM events being flushed by DBM integrations
			DefaultInputChanSize: 500,
		},
		{
			EventType:              "dbm-metrics",
			Category:               "DBM",
			ContentType:            logshttp.JSONContentType,
			EndpointsConfigPrefix:  "database_monitoring.metrics.",
			HostnameEndpointPrefix: "dbm-metrics-intake.",
			IntakeTrackType:        "dbmmetrics",
			// raise the default batch_max_concurrent_send from 0 to 10 to ensure this pipeline is able to handle 4k events/s
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    20e6,
			DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			// High input chan size is needed to handle high number of DBM events being flushed by DBM integrations
			DefaultInputChanSize: 500,
		},
		{
			EventType:   "dbm-metadata",
			ContentType: logshttp.JSONContentType,
			// set the endpoint config to "metrics" since metadata will hit the same endpoint
			// as metrics, so there is no need to add an extra config endpoint.
			// As a follow-on PR, we should clean this up to have a single config for each track type since
			// all of our data now flows through the same intake
			EndpointsConfigPrefix:  "database_monitoring.metrics.",
			HostnameEndpointPrefix: "dbm-metrics-intake.",
			IntakeTrackType:        "dbmmetadata",
			// raise the default batch_max_concurrent_send from 0 to 10 to ensure this pipeline is able to handle 4k events/s
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    20e6,
			DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			// High input chan size is needed to handle high number of DBM events being flushed by DBM integrations
			DefaultInputChanSize: 500,
		},
		{
			EventType:              "dbm-activity",
			Category:               "DBM",
			ContentType:            logshttp.JSONContentType,
			EndpointsConfigPrefix:  "database_monitoring.activity.",
			HostnameEndpointPrefix: "dbm-metrics-intake.",
			IntakeTrackType:        "dbmactivity",
			// raise the default batch_max_concurrent_send from 0 to 10 to ensure this pipeline is able to handle 4k events/s
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    20e6,
			DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			// High input chan size is needed to handle high number of DBM events being flushed by DBM integrations
			DefaultInputChanSize: 500,
		},
		{
			EventType:   "dbm-health",
			ContentType: logshttp.JSONContentType,
			// set the endpoint config to "metrics" since health will hit the same endpoint
			// as metrics, so there is no need to add an extra config endpoint.
			EndpointsConfigPrefix:  "database_monitoring.metrics.",
			HostnameEndpointPrefix: "dbm-metrics-intake.",
			IntakeTrackType:        "dbmhealth",
			// raise the default batch_max_concurrent_send from 0 to 10 to ensure this pipeline is able to handle 4k events/s
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    20e6,
			DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			// High input chan size is needed to handle high number of DBM events being flushed by DBM integrations
			DefaultInputChanSize: 500,
		},
		{
			EventType:   "dbm-column-statistics",
			ContentType: logshttp.JSONContentType,
			// set the endpoint config to "metrics" since column statistics will hit the same endpoint
			// as metrics, so there is no need to add an extra config endpoint.
			EndpointsConfigPrefix:  "database_monitoring.metrics.",
			HostnameEndpointPrefix: "dbm-metrics-intake.",
			IntakeTrackType:        "dbmcolumnstatistics",
			// raise the default batch_max_concurrent_send from 0 to 10 to ensure this pipeline is able to handle 4k events/s
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    20e6,
			DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			// High input chan size is needed to handle high number of DBM events being flushed by DBM integrations
			DefaultInputChanSize: 500,
		},
	}
}
