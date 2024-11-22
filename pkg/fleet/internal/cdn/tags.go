// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cdn

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/hosttags"
	detectenv "github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/fleet/env"
)

type hostTagsGetter struct {
	config     model.Config
	staticTags []string
}

func newHostTagsGetter(env *env.Env) hostTagsGetter {
	config := pkgconfigsetup.Datadog()
	detectenv.DetectFeatures(config)
	return hostTagsGetter{
		config:     config,
		staticTags: env.Tags,
	}
}

func (h *hostTagsGetter) get() []string {
	// Host tags are cached on host, but we add a timeout to avoid blocking the request
	// if the host tags are not available yet and need to be fetched
	ctx, cc := context.WithTimeout(context.Background(), time.Second)
	defer cc()
	hostTags := hosttags.Get(ctx, true, h.config)

	tags := []string{}
	tags = append(tags, h.staticTags...)
	tags = append(tags, hostTags.System...)
	tags = append(tags, hostTags.GoogleCloudPlatform...)
	tagSet := make(map[string]struct{})
	for _, tag := range tags {
		tagSet[tag] = struct{}{}
	}
	deduplicatedTags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		deduplicatedTags = append(deduplicatedTags, tag)
	}
	tags = deduplicatedTags

	return tags
}
