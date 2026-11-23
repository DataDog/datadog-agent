// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package remotenoopimpl provides the no-op remote issue loader.
package remotenoopimpl

import (
	"context"

	remotedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/remote/def"
)

type noopRemoteIssueLoader struct{}

// Provides defines the output of the no-op remote issue loader component.
type Provides struct {
	Comp remotedef.Component
}

// NewComponent creates a no-op remote issue loader.
func NewComponent() Provides {
	return Provides{Comp: &noopRemoteIssueLoader{}}
}

func (*noopRemoteIssueLoader) Load(context.Context) ([]remotedef.Issue, error) {
	return nil, nil
}
