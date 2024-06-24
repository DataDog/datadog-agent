// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package rdnsquerierimpl implements the rdnsquerier component interface
package rdnsquerierimpl

import (
	"context"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
)

// Requires defines the dependencies for the rdnsquerier component
type Requires struct {
	Lifecycle compdef.Lifecycle
	Config    config.Component
	Logger    log.Component
}

// Provides defines the output of the rdnsquerier component
type Provides struct {
	Comp rdnsquerier.Component
}

type rdnsQuerierImpl struct {
	config config.Component
	logger log.Component
}

// NewComponent creates a new rdnsquerier component
func NewComponent(reqs Requires) (Provides, error) {
	q := &rdnsQuerierImpl{
		config: reqs.Config,
		logger: reqs.Logger,
	}

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: q.start,
		OnStop:  q.stop,
	})

	return Provides{
		Comp: q,
	}, nil
}

func (q *rdnsQuerierImpl) start(context.Context) error {
	return nil
}

func (q *rdnsQuerierImpl) stop(context.Context) error {
	return nil
}

// GetHostname gets the hostname for the given IP address if the IP address is in the private address space, or returns an empty string if not.
// The initial implementation always returns an empty string.
func (q *rdnsQuerierImpl) GetHostname(_ []byte) string {
	return ""
}
