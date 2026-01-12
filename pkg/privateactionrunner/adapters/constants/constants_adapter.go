// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package constants

const JwtHeaderName = "X-Datadog-OnPrem-JWT"
const ModeHeaderName = "X-Datadog-OnPrem-Modes"
const VersionHeaderName = "X-Datadog-OnPrem-Version"

// HTTP Connection Constants
var (
	BaseUrlTokenName         = "base_url"
	BodyGroupName            = "body"
	BodyContentTokenName     = "content"
	BodyContentTypeTokenName = "content_type"
	UrlParametersGroupName   = "url_parameters"
	HeadersGroupName         = "headers"
)
