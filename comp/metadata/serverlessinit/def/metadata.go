// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverlessinit

// Application-runtime names reported in the runtime field. Platform mappings
// and wrapped-command detection must both use these values.
const (
	RuntimeNodeJS = "Node.js"
	RuntimePython = "Python"
	RuntimeJava   = "Java"
	RuntimeDotNet = ".NET"
	RuntimePHP    = "PHP"
	RuntimeRuby   = "Ruby"
)

// CloudServiceMetadata holds the per-platform fields of the metadata payload.
// An empty string means unavailable and is serialized as JSON null. No payload
// is sent while ResourceID is empty.
type CloudServiceMetadata struct {
	WorkloadType string

	// ResourceID is the Canonical Cloud Resource ID (CCRID) of the resource.
	// MicroVM uses the platform instance ID instead. Empty means the identity is
	// not known yet.
	ResourceID string

	// ResourceName is the platform display name (app / service / job); it is
	// never substituted with dd_service.
	ResourceName string

	Region              string
	GCPProjectID        string
	AWSAccountID        string
	AzureSubscriptionID string
	AzureResourceGroup  string

	// RuntimeCandidates are raw application-runtime values in priority order.
	// The runtime resolver normalizes them and selects the first usable value.
	// Platform-derived candidates use the runtime names declared above; values
	// read verbatim from platform metadata keep their own naming and case.
	RuntimeCandidates []string

	// ParentResourceID is the CCRID of the semantic parent (e.g. the Cloud Run
	// service behind a revision), not necessarily a string prefix of ResourceID.
	// Empty when the workload has no distinct parent.
	ParentResourceID string
}

// CloudServiceMetadataProvider supplies the platform metadata. GetMetadata must
// not perform I/O and must support concurrent reads while identity changes.
type CloudServiceMetadataProvider interface {
	// CanCollectMetadata reports platform support without collecting data or checking identity readiness.
	CanCollectMetadata() bool
	// GetMetadata returns the per-platform serverless fields for the
	// metadata payload.
	GetMetadata() CloudServiceMetadata
}
