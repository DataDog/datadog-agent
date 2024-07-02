// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package compression provides compression for trace payloads
package compression

import "io"

// team: agent-apm

// Component is the component type.
type Component interface {
	NewWriter(w io.Writer) (io.WriteCloser, error)
	NewReader(w io.Reader) (io.ReadCloser, error)
	Encoding() string
}
