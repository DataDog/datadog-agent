// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !kubeapiserver

package autoconnections

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

type stubTagsProvider struct {
	cfg model.Reader
}

// NewTagsProvider creates a TagsProvider for building connection tags
func NewTagsProvider(cfg model.Reader) TagsProvider {
	return &stubTagsProvider{cfg: cfg}
}

func (p *stubTagsProvider) GetTags(ctx context.Context, runnerID, hostname string) []string {
	// Only return basic tags in non-Kubernetes builds
	return []string{
		"runner-id:" + runnerID,
		"hostname:" + hostname,
	}
}
