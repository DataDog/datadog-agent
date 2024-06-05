// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package main

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline"
	"github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/logsagentpipelineimpl"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/datadogexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/logsagentexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/statsprocessor"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
)

const (
	_ = metricsclient.ExporterSourceTag
)

func _(
	_ datadogexporter.Config,
	_ statsprocessor.TraceAgent,
	_ config.Component,
	_ hostnameinterface.Component,
	_ log.Component,
	_ logsagentpipeline.Component,
	_ logsagentpipelineimpl.Agent,
	_ logsagentexporter.Config,
	_ model.Config,
	_ setup.ConfigurationProviders,
	_ trace.Trace,
	_ api.OTLPReceiver,
) {
}

func main() {}
