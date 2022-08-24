// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package launchers

import (
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
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
// The *sources.LogSources type satisfies this interface.
type SourceProvider interface {
	// SubscribeForType returns channels containing the new added and removed sources matching the provided type.
	//
	// Removed sources are pointer-equal to previously-added sources.
	//
	// Sources are not automatically removed when the agent shuts down.  Consumers should handle any
	// required shutdwon of running sources in their own Stop methods.
	SubscribeForType(sourceType string) (added chan *sources.LogSource, removed chan *sources.LogSource)

	// SubscribeAll returns channels containing all added and removed sources.
	SubscribeAll() (added chan *sources.LogSource, removed chan *sources.LogSource)

	// GetAddedForType returns channels containing the new added sources matching the provided type.
	GetAddedForType(sourceType string) chan *sources.LogSource
}
