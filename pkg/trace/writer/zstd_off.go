// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !zstd

package writer

import (
	"io"
)

const zstdAvailable = false

func newZstdWriter(w io.Writer) io.WriteCloser {
	return nil
}

func newZstdReader(w io.Reader) io.ReadCloser {
	return nil
}
