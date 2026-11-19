// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hosttags

import (
	"slices"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const taskARNTagPrefix = tags.TaskARN + ":"

// GlobalTagger returns the tags that apply to everything the agent reports. tagger.Component
// implements it.
type GlobalTagger interface {
	GlobalTags(cardinality types.TagCardinality) ([]string, error)
}

// PayloadTags provides the host tags to attach to a payload.
//
// On ECS Fargate there's no host, so it adds the tagger's global tags, which are meant for when
// there are no host tags. At orchestrator cardinality they include task_arn, which identifies the
// task.
type PayloadTags struct {
	provider          *HostTagProvider
	tagger            GlobalTagger
	ecsFargate        bool
	logMissingTaskARN bool
	logLimit          *log.Limit
}

// NewPayloadTags returns a PayloadTags that adds the tagger's global tags when the agent runs on
// ECS Fargate. Set logMissingTaskARN when the payload has no other way to identify the task, such
// as an empty hostname.
func NewPayloadTags(provider *HostTagProvider, tagger GlobalTagger, logMissingTaskARN bool) *PayloadTags {
	return &PayloadTags{
		provider:          provider,
		tagger:            tagger,
		ecsFargate:        fargate.GetOrchestrator() == fargate.ECS,
		logMissingTaskARN: logMissingTaskARN,
		logLimit:          log.NewLogLimit(1, 10*time.Minute),
	}
}

// Get returns the host tags for a payload, sorted and deduplicated. The slice is a copy owned by
// the caller.
func (p *PayloadTags) Get() []string {
	hostTags := slices.Clone(p.provider.GetHostTags())
	if p.ecsFargate {
		globalTags, err := p.tagger.GlobalTags(types.OrchestratorCardinality)
		if err != nil {
			log.Debugf("unable to get global tags: %v", err)
		}
		// the limiter is checked last, so it's only spent when task_arn is missing
		if p.logMissingTaskARN &&
			!slices.ContainsFunc(globalTags, func(tag string) bool { return strings.HasPrefix(tag, taskARNTagPrefix) }) &&
			p.logLimit.ShouldLog() {
			// expected briefly after startup, until the tagger has the task's metadata
			log.Infof("no task_arn in the global tags yet: payloads can't be attributed to the ECS task until it's available")
		}
		hostTags = append(hostTags, globalTags...)
	}
	// sorted and deduplicated, so encoding the slice doesn't modify it
	slices.Sort(hostTags)
	return slices.Compact(hostTags)
}
