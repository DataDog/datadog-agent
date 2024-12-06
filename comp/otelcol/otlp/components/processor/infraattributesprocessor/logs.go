// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"

	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

type infraAttributesLogProcessor struct {
	logger      *zap.Logger
	tagger      taggerClient
	cardinality types.TagCardinality
	generateID  GenerateKubeMetadataEntityID
}

func newInfraAttributesLogsProcessor(set processor.Settings, cfg *Config, tagger taggerClient, generateID GenerateKubeMetadataEntityID) (*infraAttributesLogProcessor, error) {
	ialp := &infraAttributesLogProcessor{
		logger:      set.Logger,
		tagger:      tagger,
		cardinality: cfg.Cardinality,
		generateID:  generateID,
	}

	set.Logger.Info("Logs Infra Attributes Processor configured")
	return ialp, nil
}

func (ialp *infraAttributesLogProcessor) processLogs(_ context.Context, ld plog.Logs) (plog.Logs, error) {
	rls := ld.ResourceLogs()
	for i := 0; i < rls.Len(); i++ {
		resourceAttributes := rls.At(i).Resource().Attributes()
		processInfraTags(ialp.logger, ialp.tagger, ialp.cardinality, ialp.generateID, resourceAttributes)
	}
	return ld, nil
}
