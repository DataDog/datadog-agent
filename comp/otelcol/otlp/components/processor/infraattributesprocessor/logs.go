// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"

	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

type infraAttributesLogProcessor struct {
	logger *zap.Logger
}

func newInfraAttributesLogsProcessor(set processor.CreateSettings, _ *Config) (*infraAttributesLogProcessor, error) {
	telp := &infraAttributesLogProcessor{
		logger: set.Logger,
	}

	set.Logger.Info("Logs Infra Attributes Processor configured")
	return telp, nil
}

func (telp *infraAttributesLogProcessor) processLogs(_ context.Context, ld plog.Logs) (plog.Logs, error) {
	return ld, nil
}
