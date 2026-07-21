// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

type infraAttributesLogProcessor struct {
	infraTags   infraTagsProcessor
	logger      *zap.Logger
	cardinality types.TagCardinality
	cfg         *Config
}

func newInfraAttributesLogsProcessor(
	set processor.Settings,
	infraTags infraTagsProcessor,
	cfg *Config,
) (*infraAttributesLogProcessor, error) {
	ialp := &infraAttributesLogProcessor{
		infraTags:   infraTags,
		logger:      set.Logger,
		cardinality: cfg.Cardinality,
		cfg:         cfg,
	}
	set.Logger.Info("Logs Infra Attributes Processor configured")
	return ialp, nil
}

func (ialp *infraAttributesLogProcessor) processLogs(_ context.Context, ld plog.Logs) (plog.Logs, error) {
	rls := ld.ResourceLogs()
	for i := 0; i < rls.Len(); i++ {
		rl := rls.At(i)
		resourceAttributes := rl.Resource().Attributes()

		var ddtags []string
		var ddtagsSink *[]string
		if ialp.cfg.LogsTagsAsDDTags {
			ddtagsSink = &ddtags
		}

		// trace_container_tag_promotion only makes sense for traces: it exists to
		// feed trace-agent's `_dd.tags.container` promotion
		// (ConsumeContainerTagsFromResource), which logs never go through.
		// Always pass "off" here regardless of the configured mode.
		ialp.infraTags.ProcessTags(ialp.logger, ialp.cardinality, resourceAttributes, ialp.cfg.AllowHostnameOverride, ContainerTagPromotionOff, ddtagsSink)

		if len(ddtags) == 0 {
			continue
		}
		newTags := strings.Join(ddtags, ",")
		writeDdtagsToLogRecords(rl, newTags)
	}
	return ld, nil
}

// writeDdtagsToLogRecords appends newTags to the `ddtags` attribute of every
// log record under rl, merging with any value the record already carries
// rather than overwriting it.
func writeDdtagsToLogRecords(rl plog.ResourceLogs, newTags string) {
	sls := rl.ScopeLogs()
	for i := 0; i < sls.Len(); i++ {
		lrs := sls.At(i).LogRecords()
		for j := 0; j < lrs.Len(); j++ {
			attrs := lrs.At(j).Attributes()
			if existing, ok := attrs.Get("ddtags"); ok && existing.AsString() != "" {
				attrs.PutStr("ddtags", existing.AsString()+","+newTags)
			} else {
				attrs.PutStr("ddtags", newTags)
			}
		}
	}
}
