// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventplatformimpl

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
)

// These mirror the logs-platform defaults (pkg/config/setup DefaultBatch*/DefaultInputChanSize),
// inlined to avoid importing pkg/config/setup from inside comp/ (depguard pkgconfigusage).
const (
	sdsDefaultBatchMaxContentSize = 5000000
	sdsDefaultBatchMaxSize        = 1000
	sdsDefaultInputChanSize       = 100
)

func getDataSecurityPipelines() []passthroughPipelineDesc {
	return []passthroughPipelineDesc{
		{
			eventType:                     eventplatform.EventTypeSDSResult,
			category:                      "Data Security",
			contentType:                   logshttp.ProtobufContentType,
			endpointsConfigPrefix:         "sds_result.forwarder.",
			hostnameEndpointPrefix:        "sds-intake.",
			intakeTrackType:               "sdsresult",
			defaultBatchMaxConcurrentSend: 10,
			defaultBatchMaxContentSize:    sdsDefaultBatchMaxContentSize,
			defaultBatchMaxSize:           sdsDefaultBatchMaxSize,
			defaultInputChanSize:          sdsDefaultInputChanSize,
		},
	}
}
