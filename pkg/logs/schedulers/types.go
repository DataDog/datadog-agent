// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package schedulers

import (
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// Scheduler implementations manage logs-agent sources.
//
// Schedulers are started when the logs-agent starts, and stopped when it stops.
type Scheduler interface {
	// Start the scheduler, managing sources and services via the given manager.
	Start(sourceMgr SourceManager)

	// Stop the scheduler, and wait until shutdown is complete.  It is not
	// necessary to remove sources or services here, but any background
	// goroutines or other resources should be freed.
	Stop()
}

// SourceManager is the interface by which schedulers add and remove sources from the agent.
//
// (services are also included here, temporarily)
type SourceManager interface {
	// AddSource adds a new source to the logs agent.  The new source is
	// distributed to all active launchers, which may create appropriate
	// tailers and begin forwarding messages.
	AddSource(source *sources.LogSource)

	// RemoveSource removes an existing source from the logs agent.  The
	// source is recognized by pointer equality.
	RemoveSource(source *sources.LogSource)

	// GetSources returns all the sources currently held.  The result is copied and
	// will not be modified after it is returned, and represents a "snapshot" of the
	// state when the function was called.
	GetSources() []*sources.LogSource

	// AddService adds a new service to the logs agent.
	AddService(service *service.Service)

	// RemoveService removes a service added with AddService.  The source
	// is recognized by pointer equality.
	RemoveService(service *service.Service)
}
