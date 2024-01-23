// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package logsagentexporter contains a logs exporter which forwards logs to a channel.
package logsagentexporter

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"go.opentelemetry.io/collector/component"
	exp "go.opentelemetry.io/collector/exporter"
)

const (
	// TypeStr defines the logsagent exporter type string.
	TypeStr       = "logsagent"
	stability     = component.StabilityLevelStable
	logSourceName = "OTLP log ingestion"
)

type factory struct {
	logsAgentChannel chan *message.Message
}

// NewFactory creates a new logsagentexporter factory.
func NewFactory(logsAgentChannel chan *message.Message) exp.Factory {
	panic("not called")
}

func (f *factory) createLogsExporter(
	ctx context.Context,
	set exp.CreateSettings,
	c component.Config,
) (exp.Logs, error) {
	panic("not called")
}
