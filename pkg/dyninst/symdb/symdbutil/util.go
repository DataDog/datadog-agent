// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package symdbutil provides utility functions and types for the symdb package.
package symdbutil

import (
	"io"
)

// PanickingWriter wraps an io.StringWriter, but writes panic instead of
// returning errors.
//
// PanickingWriter implements symdb.StringWriter.
type PanickingWriter struct {
	inner io.StringWriter
}

// MakePanickingWriter creates a PanickingWriter that will write to the provided
// writer.
func MakePanickingWriter(w io.StringWriter) PanickingWriter {
	return PanickingWriter{inner: w}
}

// WriteString implements symdb.StringWriter.
func (w PanickingWriter) WriteString(s string) {
	_, err := w.inner.WriteString(s)
	if err != nil {
		panic(err)
	}
}
