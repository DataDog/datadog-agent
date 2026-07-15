// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package setup

// OTLP configuration paths.
const (
	OTLPSection                              = "otlp_config"
	OTLPTracePort                            = OTLPSection + ".traces.internal_port"
	OTLPTracesEnabled                        = OTLPSection + ".traces.enabled"
	OTLPTracesInfraAttrEnabled               = OTLPSection + ".traces.infra_attributes.enabled"
	OTLPTracesInfraAttrContainerTagPromotion = OTLPSection + ".traces.infra_attributes.container_tag_promotion"

	OTLPLogs        = OTLPSection + ".logs"
	OTLPLogsEnabled = OTLPLogs + ".enabled"

	OTLPReceiverSubSectionKey = "receiver"
	OTLPReceiverSection       = OTLPSection + "." + OTLPReceiverSubSectionKey

	OTLPMetrics        = OTLPSection + ".metrics"
	OTLPMetricsEnabled = OTLPMetrics + ".enabled"
	OTLPMetricsBatch   = OTLPMetrics + ".batch"

	OTLPDebug = OTLPSection + "." + "debug"

	DataPlaneSection     = "data_plane"
	DataPlaneEnabled     = DataPlaneSection + ".enabled"
	DataPlaneOTLPSection = DataPlaneSection + ".otlp"
	DataPlaneOTLPEnabled = DataPlaneOTLPSection + ".enabled"

	DataPlaneOTLPProxySection = DataPlaneOTLPSection + ".proxy"
	DataPlaneOTLPProxyEnabled = DataPlaneOTLPProxySection + ".enabled"

	DataPlaneOTLPProxyReceiverSection               = DataPlaneOTLPProxySection + ".receiver"
	DataPlaneOTLPProxyReceiverProtocolsGRPCEndpoint = DataPlaneOTLPProxyReceiverSection + ".protocols.grpc.endpoint"
)
