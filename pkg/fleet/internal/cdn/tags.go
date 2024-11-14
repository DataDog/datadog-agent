// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cdn

import (
	"context"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/hosttags"
	detectenv "github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/fleet/env"
)

type hostTagsGetter struct {
	config model.Config
}

func newHostTagsGetter(env *env.Env) hostTagsGetter {
	config := model.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	detectenv.DetectFeatures(config)
	config.Set("tags", env.Tags, model.SourceFile)
	return hostTagsGetter{
		config: config,
	}
}

func (h *hostTagsGetter) get() []string {
	// Host tags are cached on host, but we add a timeout to avoid blocking the request
	// if the host tags are not available yet and need to be fetched
	ctx, cc := context.WithTimeout(context.Background(), time.Second)
	defer cc()
	hostTags := hosttags.Get(ctx, true, h.config)
	tags := append(hostTags.System, hostTags.GoogleCloudPlatform...)
	return tags
}
