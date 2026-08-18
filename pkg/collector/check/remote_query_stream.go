// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package check

// RemoteQueryStreamEvent is the binary-safe event emitted by Remote Queries COPY stream helpers.
type RemoteQueryStreamEvent struct {
	Type         string
	MetadataJSON string
	Payload      []byte
}
