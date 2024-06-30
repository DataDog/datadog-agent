// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package endpoints stores a collection of `transaction.Endpoint` mainly used by the forwarder package to send data to
// Datadog using the right request path for a given type of data.
package endpoints

import "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"

var (
	// V1SeriesEndpoint is a v1 endpoint used to send series
	V1SeriesEndpoint = transaction.Endpoint{Subdomain: "", Route: "/api/v1/series", Name: "series_v1"}
	// V1CheckRunsEndpoint is a v1 endpoint used to send checks results
	V1CheckRunsEndpoint = transaction.Endpoint{Subdomain: "", Route: "/api/v1/check_run", Name: "check_run_v1"}
	// V1IntakeEndpoint is a v1 endpoint, used by Agent v.5, still used for metadata
	V1IntakeEndpoint = transaction.Endpoint{Subdomain: "", Route: "/intake/", Name: "intake"}
	// V1ValidateEndpoint is a v1 endpoint used to validate API keys
	V1ValidateEndpoint = transaction.Endpoint{Subdomain: "", Route: "/api/v1/validate", Name: "validate_v1"}
	// V1MetadataEndpoint is a v1 endpoint used for metadata (only used for inventory metadata for now)
	V1MetadataEndpoint = transaction.Endpoint{Subdomain: "", Route: "/api/v1/metadata", Name: "metadata_v1"}
	// SeriesEndpoint is the v2 endpoint used to send series
	SeriesEndpoint = transaction.Endpoint{Subdomain: "", Route: "/api/v2/series", Name: "series_v2"}
	// SketchSeriesEndpoint is the v2 endpoint used to send sketches
	SketchSeriesEndpoint = transaction.Endpoint{Subdomain: "", Route: "/api/beta/sketches", Name: "sketches_v2"}

	// ProcessStatusEndpoint is a v1 endpoint used to send process checks
	ProcessStatusEndpoint = transaction.Endpoint{Subdomain: "process", Route: "/status", Name: "process_status"}
	// ProcessesIntakeStatusEndpoint is a v1 endpoint used to send processes checks
	ProcessesIntakeStatusEndpoint = transaction.Endpoint{Subdomain: "process", Route: "/intake/status", Name: "process_intake_status"}
	// ProcessesEndpoint is a v1 endpoint used to send processes checks
	ProcessesEndpoint = transaction.Endpoint{Subdomain: "process", Route: "/api/v1/collector", Name: "process"} // work with processes subdomain (get 403)
	// ProcessDiscoveryEndpoint is a v1 endpoint used to sends process discovery checks
	ProcessDiscoveryEndpoint = transaction.Endpoint{Subdomain: "process", Route: "/api/v1/discovery", Name: "process_discovery"} // work with processes subdomain (get 403)
	// ProcessLifecycleEndpoint is a v2 endpoint used to send process lifecycle events
	ProcessLifecycleEndpoint = transaction.Endpoint{Subdomain: "orchestrator", Route: "/api/v2/proclcycle", Name: "process_lifecycle"} // 404 not found
	// RtProcessesEndpoint is a v1 endpoint used to send real time process checks
	RtProcessesEndpoint = transaction.Endpoint{Subdomain: "process", Route: "/api/v1/collector", Name: "rtprocess"} // work with processes subdomain (get 403)
	// ContainerEndpoint is a v1 endpoint used to send container checks
	ContainerEndpoint = transaction.Endpoint{Subdomain: "process", Route: "/api/v1/container", Name: "container"} // work with processes subdomain (get 403)
	// RtContainerEndpoint is a v1 endpoint used to send real time container checks
	RtContainerEndpoint = transaction.Endpoint{Subdomain: "process", Route: "/api/v1/container", Name: "rtcontainer"}
	// ConnectionsEndpoint is a v1 endpoint used to send connection checks
	ConnectionsEndpoint = transaction.Endpoint{Subdomain: "process", Route: "/api/v1/connections", Name: "connections"}
	// LegacyOrchestratorEndpoint is a v1 endpoint used to send orchestrator checks
	LegacyOrchestratorEndpoint = transaction.Endpoint{Subdomain: "orchestrator", Route: "/api/v1/orchestrator", Name: "orchestrator"}
	// OrchestratorEndpoint is a v2 endpoint used to send orchestrator checks
	OrchestratorEndpoint = transaction.Endpoint{Subdomain: "orchestrator", Route: "/api/v2/orch", Name: "orchestrator"}
	// OrchestratorManifestEndpoint is a v2 endpoint used to send orchestrator manifests
	OrchestratorManifestEndpoint = transaction.Endpoint{Subdomain: "orchestrator", Route: "/api/v2/orchmanif", Name: "orchmanifest"} // work with POST but got 401

	/////////////// Unused Endpoints ///////////////

	// V1SketchSeriesEndpoint is a v1 endpoint used to send sketches
	V1SketchSeriesEndpoint = transaction.Endpoint{Subdomain: "", Route: "/api/v1/sketches", Name: "sketches_v1"} //nolint
	// EventsEndpoint is the v2 endpoint used to send events
	EventsEndpoint = transaction.Endpoint{Subdomain: "", Route: "/api/v2/events", Name: "events_v2"}
	// ServiceChecksEndpoint is the v2 endpoint used to send service checks
	ServiceChecksEndpoint = transaction.Endpoint{Subdomain: "", Route: "/api/v2/service_checks", Name: "services_checks_v2"}
	// HostMetadataEndpoint is the v2 endpoint used to send host medatada
	HostMetadataEndpoint = transaction.Endpoint{Subdomain: "", Route: "/api/v2/host_metadata", Name: "host_metadata_v2"}
)
