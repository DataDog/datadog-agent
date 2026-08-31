// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package check

// RemoteQueryStreamEvent is the metadata-only event emitted by Remote Queries
// stream helpers. Bulk result bytes never cross the bridge: the integration
// uploads bounded JSON page files itself, so the stream carries only progress
// metadata, the final compact run receipt, and errors.
type RemoteQueryStreamEvent struct {
	Type         string
	MetadataJSON string
}
