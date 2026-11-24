// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fargate

import (
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GlobalTagger returns the tags that apply to everything the Agent reports. tagger.Component
// implements it.
type GlobalTagger interface {
	GlobalTags(cardinality types.TagCardinality) ([]string, error)
}

// ECSTaskTags returns the tags of the ECS Fargate task the Agent runs in, and whether it runs on
// ECS Fargate at all. Outside of ECS Fargate it returns nil and false.
//
// They're the tagger's global tags at orchestrator cardinality, which include task_arn. Payloads
// sent without a hostname can carry them to identify the task.
func ECSTaskTags(tagger GlobalTagger) ([]string, bool) {
	return ecsTaskTags(GetOrchestrator(), tagger)
}

func ecsTaskTags(orchestrator OrchestratorName, tagger GlobalTagger) ([]string, bool) {
	if orchestrator != ECS {
		return nil, false
	}
	taskTags, err := tagger.GlobalTags(types.OrchestratorCardinality)
	if err != nil {
		log.Debugf("unable to get the ECS task tags: %v", err)
	}
	return taskTags, true
}

// HasTaskARN reports whether tagList includes a task_arn tag.
func HasTaskARN(tagList []string) bool {
	return slices.ContainsFunc(tagList, func(tag string) bool { return strings.HasPrefix(tag, tags.TaskARN+":") })
}
