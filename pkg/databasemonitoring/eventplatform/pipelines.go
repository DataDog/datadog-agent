// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eventplatform contains Database Monitoring event-platform pipeline descriptors.
package eventplatform

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

const (
	EventTypeDBMSamples          = "dbm-samples"
	EventTypeDBMMetrics          = "dbm-metrics"
	EventTypeDBMActivity         = "dbm-activity"
	EventTypeDBMMetadata         = "dbm-metadata"
	EventTypeDBMHealth           = "dbm-health"
	EventTypeDBMColumnStatistics = "dbm-column-statistics"
)

// Pipelines returns the Database Monitoring event-platform pipelines.
func Pipelines() []eventplatform.PipelineDesc {
	return []eventplatform.PipelineDesc{
		{
			EventType:                     EventTypeDBMSamples,
			Category:                      "DBM",
			ContentType:                   eventplatform.ContentTypeJSON,
			EndpointsConfigPrefix:         "database_monitoring.samples.",
			HostnameEndpointPrefix:        "dbm-metrics-intake.",
			IntakeTrackType:               "databasequery",
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    10e6,
			DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			DefaultInputChanSize:          500,
		},
		{
			EventType:                     EventTypeDBMMetrics,
			Category:                      "DBM",
			ContentType:                   eventplatform.ContentTypeJSON,
			EndpointsConfigPrefix:         "database_monitoring.metrics.",
			HostnameEndpointPrefix:        "dbm-metrics-intake.",
			IntakeTrackType:               "dbmmetrics",
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    20e6,
			DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			DefaultInputChanSize:          500,
		},
		{
			EventType:                     EventTypeDBMMetadata,
			ContentType:                   eventplatform.ContentTypeJSON,
			EndpointsConfigPrefix:         "database_monitoring.metrics.",
			HostnameEndpointPrefix:        "dbm-metrics-intake.",
			IntakeTrackType:               "dbmmetadata",
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    20e6,
			DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			DefaultInputChanSize:          500,
		},
		{
			EventType:                     EventTypeDBMActivity,
			Category:                      "DBM",
			ContentType:                   eventplatform.ContentTypeJSON,
			EndpointsConfigPrefix:         "database_monitoring.activity.",
			HostnameEndpointPrefix:        "dbm-metrics-intake.",
			IntakeTrackType:               "dbmactivity",
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    20e6,
			DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			DefaultInputChanSize:          500,
		},
		{
			EventType:                     EventTypeDBMHealth,
			ContentType:                   eventplatform.ContentTypeJSON,
			EndpointsConfigPrefix:         "database_monitoring.metrics.",
			HostnameEndpointPrefix:        "dbm-metrics-intake.",
			IntakeTrackType:               "dbmhealth",
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    20e6,
			DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			DefaultInputChanSize:          500,
		},
		{
			EventType:                     EventTypeDBMColumnStatistics,
			ContentType:                   eventplatform.ContentTypeJSON,
			EndpointsConfigPrefix:         "database_monitoring.metrics.",
			HostnameEndpointPrefix:        "dbm-metrics-intake.",
			IntakeTrackType:               "dbmcolumnstatistics",
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    20e6,
			DefaultBatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
			DefaultInputChanSize:          500,
		},
	}
}
