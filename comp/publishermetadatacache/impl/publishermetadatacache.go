// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

// Package publishermetadatacacheimpl implements the publishermetadatacache component interface.
package publishermetadatacacheimpl

import (
	"context"

	publishermetadatacache "github.com/DataDog/datadog-agent/comp/publishermetadatacache/def"
	publishermetadatacachepkg "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/publishermetadatacache"

	compdef "github.com/DataDog/datadog-agent/comp/def"

	winevtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/windows"
)

// Requires defines the dependencies for the publishermetadatacache component
type Requires struct {
	Lifecycle compdef.Lifecycle
}

// Provides defines the output of the publishermetadatacache component
type Provides struct {
	Comp publishermetadatacache.Component
}

// NewComponent creates a new publishermetadatacache component
func NewComponent(reqs Requires) Provides {
	cache := publishermetadatacachepkg.New(winevtapi.New())

	// Register cleanup hook to close all handles when component shuts down
	reqs.Lifecycle.Append(compdef.Hook{
		OnStop: func(_ context.Context) error {
			cache.Flush()
			return nil
		},
	})

	return Provides{
		Comp: cache,
	}
}
