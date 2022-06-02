// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package endpoints stores a collection of `transaction.Endpoint` mainly used by the forwarder package to send data to
// Datadog using the right request path for a given type of data.
package endpoints

import "github.com/DataDog/datadog-agent/pkg/forwarder/transaction"

var (
	// V1SeriesEndpoint is a v1 endpoint used to send series
	V1SeriesEndpoint = transaction.Endpoint{Route: "/api/v1/series", Name: "series_v1"}
	// V1CheckRunsEndpoint is a v1 endpoint used to send checks results
	V1CheckRunsEndpoint = transaction.Endpoint{Route: "/api/v1/check_run", Name: "check_run_v1"}
	// V1IntakeEndpoint is a v1 endpoint, used by Agent v.5, still used for metadata
	V1IntakeEndpoint = transaction.Endpoint{Route: "/intake/", Name: "intake"}
	// V1SketchSeriesEndpoint is a v1 endpoint used to send sketches
	V1SketchSeriesEndpoint = transaction.Endpoint{Route: "/api/v1/sketches", Name: "sketches_v1"} // nolint unused for now
	// V1ValidateEndpoint is a v1 endpoint used to validate API keys
	V1ValidateEndpoint = transaction.Endpoint{Route: "/api/v1/validate", Name: "validate_v1"}
	// V1MetadataEndpoint is a v1 endpoint used for metadata (only used for inventory metadata for now)
	V1MetadataEndpoint = transaction.Endpoint{Route: "/api/v1/metadata", Name: "metadata_v1"}

	// SeriesEndpoint is the v2 endpoint used to send series
	SeriesEndpoint = transaction.Endpoint{Route: "/api/v2/series", Name: "series_v2"}
	// EventsEndpoint is the v2 endpoint used to send events
	EventsEndpoint = transaction.Endpoint{Route: "/api/v2/events", Name: "events_v2"}
	// ServiceChecksEndpoint is the v2 endpoint used to send service checks
	ServiceChecksEndpoint = transaction.Endpoint{Route: "/api/v2/service_checks", Name: "services_checks_v2"}
	// SketchSeriesEndpoint is the v2 endpoint used to send sketches
	SketchSeriesEndpoint = transaction.Endpoint{Route: "/api/beta/sketches", Name: "sketches_v2"}
	// HostMetadataEndpoint is the v2 endpoint used to send host medatada
	HostMetadataEndpoint = transaction.Endpoint{Route: "/api/v2/host_metadata", Name: "host_metadata_v2"}

	// ProcessesEndpoint is a v1 endpoint used to send processes checks
	ProcessesEndpoint = transaction.Endpoint{Route: "/api/v1/collector", Name: "process"}
	// ProcessDiscoveryEndpoint is a v1 endpoint used to sends process discovery checks
	ProcessDiscoveryEndpoint = transaction.Endpoint{Route: "/api/v1/discovery", Name: "process_discovery"}
	// ProcessLifecycleEndpoint is a v2 endpoint used to send process lifecycle events
	ProcessLifecycleEndpoint = transaction.Endpoint{Route: "/api/v2/proclcycle", Name: "proclcycle"}
	// RtProcessesEndpoint is a v1 endpoint used to send real time process checks
	RtProcessesEndpoint = transaction.Endpoint{Route: "/api/v1/collector", Name: "rtprocess"}
	// ContainerEndpoint is a v1 endpoint used to send container checks
	ContainerEndpoint = transaction.Endpoint{Route: "/api/v1/container", Name: "container"}
	// RtContainerEndpoint is a v1 endpoint used to send real time container checks
	RtContainerEndpoint = transaction.Endpoint{Route: "/api/v1/container", Name: "rtcontainer"}
	// ConnectionsEndpoint is a v1 endpoint used to send connection checks
	ConnectionsEndpoint = transaction.Endpoint{Route: "/api/v1/collector", Name: "connections"}
	// LegacyOrchestratorEndpoint is a v1 endpoint used to send orchestrator checks
	LegacyOrchestratorEndpoint = transaction.Endpoint{Route: "/api/v1/orchestrator", Name: "orchestrator"}
	// OrchestratorEndpoint is a v2 endpoint used to send orchestrator checks
	OrchestratorEndpoint = transaction.Endpoint{Route: "/api/v2/orch", Name: "orchestrator"}
	// ContainerLifecycleEndpoint is an event platform endpoint used to send container lifecycle events
	ContainerLifecycleEndpoint = transaction.Endpoint{Route: "/api/v2/contlcycle", Name: "contlcycle"}
)
