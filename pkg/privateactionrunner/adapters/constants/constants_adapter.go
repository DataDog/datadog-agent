// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package constants

const (
	// InternalOpmsInsecureHostEnvVar is an internal-only env var for e2e tests.
	// When set to "true" and DD_DD_URL points at an http:// server, PAR's OPMS
	// client sends dequeue/heartbeat/result calls to that host over plain HTTP
	// instead of the production https://api.<site> endpoint. It has no effect
	// on task signature verification, which is always enforced.
	// NOT intended for customer use.
	InternalOpmsInsecureHostEnvVar = "DD_INTERNAL_PAR_OPMS_INSECURE_HOST"

	JwtHeaderName           = "X-Datadog-OnPrem-JWT"
	ModeHeaderName          = "X-Datadog-OnPrem-Modes"
	VersionHeaderName       = "X-Datadog-OnPrem-Version"
	PlatformHeaderName      = "X-Datadog-OnPrem-Platform"
	ArchitectureHeaderName  = "X-Datadog-OnPrem-Architecture"
	FlavorHeaderName        = "X-Datadog-OnPrem-Flavor"
	ContainerizedHeaderName = "X-Datadog-OnPrem-Containerized"
)

// HTTP Connection Constants
var (
	BaseUrlTokenName         = "base_url"
	BodyGroupName            = "body"
	BodyContentTokenName     = "content"
	BodyContentTypeTokenName = "content_type"
	UrlParametersGroupName   = "url_parameters"
	HeadersGroupName         = "headers"
	TestingName              = "testing"
	TestingPathName          = "path"
	TestingVerbName          = "verb"
)
