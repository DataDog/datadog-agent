// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hosttags

import (
	"slices"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const taskARNTagPrefix = "task_arn:"

// GlobalTagger returns the tags that apply to everything the agent reports. tagger.Component
// implements it.
type GlobalTagger interface {
	GlobalTags(cardinality types.TagCardinality) ([]string, error)
}

// PayloadTags provides the host tags to attach to a payload.
//
// On ECS Fargate there's no host and the payload hostname can be empty, so it adds the tagger's
// global tags, which are meant for when there are no host tags. At orchestrator cardinality they
// include task_arn, which identifies the task.
type PayloadTags struct {
	provider   *HostTagProvider
	tagger     GlobalTagger
	ecsFargate bool
	warnLimit  *log.Limit
}

// NewPayloadTags returns a PayloadTags that adds the tagger's global tags when the agent runs on
// ECS Fargate.
func NewPayloadTags(provider *HostTagProvider, tagger GlobalTagger) *PayloadTags {
	return &PayloadTags{
		provider:   provider,
		tagger:     tagger,
		ecsFargate: fargate.GetOrchestrator() == fargate.ECS,
		warnLimit:  log.NewLogLimit(1, 10*time.Minute),
	}
}

// Get returns the host tags for a payload. The returned slice is a copy the caller may modify.
func (p *PayloadTags) Get() []string {
	tags := slices.Clone(p.provider.GetHostTags())
	if !p.ecsFargate {
		return tags
	}

	globalTags, err := p.tagger.GlobalTags(types.OrchestratorCardinality)
	if err != nil {
		log.Debugf("unable to get global tags: %v", err)
	}
	if !slices.ContainsFunc(globalTags, func(tag string) bool { return strings.HasPrefix(tag, taskARNTagPrefix) }) && p.warnLimit.ShouldLog() {
		// expected briefly after startup, until the tagger has the task's metadata
		log.Warnf("no task_arn in the global tags: payloads can't be attributed to the ECS task until it's available")
	}
	return append(tags, globalTags...)
}
