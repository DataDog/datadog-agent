// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package launchers

import (
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// Launcher implementations launch logs pipelines in response to sources, and
// mange those pipelines' lifetime.
//
// Launchers are started when the logs-agent starts, or when they are added to
// the agent, and stopped when it stops.
type Launcher interface {
	// Start the launcher.
	Start(sourceProvider SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry)

	// Stop the launcher, and wait until shutdown is complete.  It is not
	// necessary to unsubscribe from the sourceProvider, but any background
	// goroutines or other resources should be freed.
	Stop()
}

// SourceProvider is the interface by which launchers subscribe to changes in sources.
//
// The *config.LogSources type satisfies this interface.
type SourceProvider interface {
	// GetAddedForType returns the new added sources matching the provided type.
	GetAddedForType(sourceType string) chan *config.LogSource

	// GetRemovedForType returns the new removed sources matching the provided type.
	GetRemovedForType(sourceType string) chan *config.LogSource
}
