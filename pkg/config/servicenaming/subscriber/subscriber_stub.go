// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !cel

// Package subscriber provides a workloadmeta subscriber that evaluates CEL-based
// service naming rules against container metadata.
package subscriber

import (
	"context"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// Subscriber is a stub for non-cel builds.
type Subscriber struct{}

// NewSubscriber returns nil when cel is not enabled.
func NewSubscriber(_ pkgconfigmodel.Reader, _ workloadmeta.Component) (*Subscriber, error) {
	return nil, nil
}

// Start is a no-op for the stub.
func (s *Subscriber) Start(_ context.Context) {}
