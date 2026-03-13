// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package observability

import (
	"context"
	"fmt"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/modes"
)

const (
	RunnerIdTagName            = "runner_id"
	RunnerVersionTagName       = "runner_version"
	ModesTagName               = "modes"
	LegacyModeWorkflowsTagName = "mode_workflows"
	ActionFqnTagName           = "action_fqn"
	ActionClientTagName        = "action_client"
	ExecutionResultTagName     = "execution_result"
	TaskIDTagName              = "task_id"
	JobIDTagName               = "job_id"

	Duration = "duration"
)

type CommonTags struct {
	RunnerId      string
	RunnerVersion string
	Modes         []modes.Mode
	ExtraTags     []Tag
}

type Tag struct {
	Key   string
	Value string
}

func (t *CommonTags) AsMetricTags() []string {
	var tags []string

	addIfNotEmpty := func(key, value string) {
		if value != "" {
			tags = append(tags, fmt.Sprintf("%s:%s", key, value))
		}
	}
	addIfNotEmpty(RunnerIdTagName, t.RunnerId)
	addIfNotEmpty(RunnerVersionTagName, t.RunnerVersion)
	pullEnabled := "true"
	addIfNotEmpty(modes.ModePull.MetricTag(), pullEnabled)

	for _, tag := range t.ExtraTags {
		addIfNotEmpty(tag.Key, tag.Value)
	}

	return tags
}

func (t *CommonTags) AsLogFields() []log.Field {
	fields := []log.Field{
		log.String(RunnerIdTagName, t.RunnerId),
		log.String(RunnerVersionTagName, t.RunnerVersion),
		log.Strings(ModesTagName, modes.ToStrings(t.Modes)),
	}
	for _, tag := range t.ExtraTags {
		fields = append(fields, log.String(tag.Key, tag.Value))
	}
	return fields
}

func AddCommonTagsToLogs(ctx context.Context, tags CommonTags) context.Context {
	logger := log.FromContext(ctx)
	return log.ContextWithLogger(ctx, logger.With(tags.AsLogFields()...))
}
