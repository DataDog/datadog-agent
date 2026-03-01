// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

//go:build !cgo

package replayimpl

import (
	"errors"
	"io"
)

func zstdDecompress(_ []byte) ([]byte, error) {
	return nil, errors.New("zstd requires CGO")
}

func newZstdWriter(w io.WriteCloser) io.WriteCloser {
	// No zstd without CGO; write uncompressed
	return w
}
