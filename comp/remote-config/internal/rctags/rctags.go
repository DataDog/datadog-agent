// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package rctags builds the host tags Remote Config clients report to the
// backend for targeting.
package rctags

import (
	"context"
	"strings"
	"time"

	cfgcomp "github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertags "github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/metadata/host/impl/hosttags"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Getter returns a function that builds the current host tags.
func Getter(config cfgcomp.Component, taggerOpt option.Option[tagger.Component]) func() []string {
	return func() []string {
		// Host tags are cached on host, but we add a timeout to avoid blocking the RC request
		// if the host tags are not available yet and need to be fetched. They will be fetched
		// by the first agent metadata V5 payload.
		ctx, cc := context.WithTimeout(context.Background(), time.Second)
		defer cc()
		hostTags := hosttags.Get(ctx, true, config, true)
		tags := append(hostTags.System, hostTags.GoogleCloudPlatform...)

		// On ECS Fargate, the task_arn tag is not part of host tags but is
		// needed for RC predicate targeting. Fetch it from the tagger's global
		// tags at orchestrator cardinality.
		if taggerComp, ok := taggerOpt.Get(); ok {
			globalTags, err := taggerComp.GlobalTags(taggertypes.OrchestratorCardinality)
			if err == nil {
				taskARNPrefix := taggertags.TaskARN + ":"
				for _, t := range globalTags {
					if strings.HasPrefix(t, taskARNPrefix) {
						tags = append(tags, t)
						break
					}
				}
			}
		}

		return tags
	}
}
