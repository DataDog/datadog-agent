// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubelet || !kubeapiserver

package hostinfo

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// KubeNodeTagsProvider allows computing node tags based on the user configurations for node labels and annotations as tags
type KubeNodeTagsProvider struct{}

// NewKubeNodeTagsProvider creates and returns a new kube node tags provider object
func NewKubeNodeTagsProvider(_ config.Reader) KubeNodeTagsProvider {
	return KubeNodeTagsProvider{}
}

// GetTags gets the tags from the kubernetes apiserver
//
//nolint:revive // TODO(CINT) Fix revive linter
func (k KubeNodeTagsProvider) GetTags(_ context.Context) ([]string, error) {
	return nil, nil
}
