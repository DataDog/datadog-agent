// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package hostname

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

type service struct{}

var _ Component = (*service)(nil)

// Get returns the hostname.
func (hs *service) Get(ctx context.Context) (string, error) {
	return hostname.Get(ctx)
}

// GetSafe returns the hostname, or 'unknown host' if anything goes wrong.
func (hs *service) GetSafe(ctx context.Context) string {
	name, err := hs.Get(ctx)
	if err != nil {
		return "unknown host"
	}
	return name
}

// GetWithProvider returns the hostname for the Agent and the provider that was use to retrieve it.
func (hs *service) GetWithProvider(ctx context.Context) (hostname.Data, error) {
	return hostname.GetWithProvider(ctx)
}

// newHostnameService fetches the hostname and returns a service wrapping it
func newHostnameService() Component {
	return &service{}
}
