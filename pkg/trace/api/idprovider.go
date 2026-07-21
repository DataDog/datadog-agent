// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

// IDProvider implementations can look up a container ID given a context and http header.
// When HasContainerFeatures is false, use NoopContainerIDProvider() to avoid reading headers or calling the tagger.
type IDProvider interface {
	GetContainerID(context.Context, http.Header) string
}

// noopContainerIDProvider always returns "" without reading headers or calling ContainerIDFromOriginInfo.
type noopContainerIDProvider struct{}

// GetContainerID returns an empty string. It does not read HTTP headers or call ContainerIDFromOriginInfo.
func (*noopContainerIDProvider) GetContainerID(_ context.Context, _ http.Header) string {
	return ""
}

// NoopContainerIDProvider returns an IDProvider that always returns "".
// Use it when HasContainerFeatures is false to avoid unnecessary header parsing and cgroup/tagger work.
func NoopContainerIDProvider() IDProvider {
	return &noopContainerIDProvider{}
}

// NewContainerIDProviderFromConfig returns an IDProvider based on the trace config.
// When HasContainerFeatures is false, it returns a noop provider that does not read headers or call the tagger.
// Otherwise it returns the standard provider that uses ContainerProcRoot and ContainerIDFromOriginInfo.
func NewContainerIDProviderFromConfig(conf *config.AgentConfig) IDProvider {
	if !conf.HasContainerFeatures {
		return NoopContainerIDProvider()
	}
	return NewIDProvider(conf.ContainerProcRoot, conf.ContainerIDFromOriginInfo)
}
