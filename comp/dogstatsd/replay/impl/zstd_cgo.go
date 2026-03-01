// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

//go:build cgo

package replayimpl

import (
	"io"

	"github.com/DataDog/zstd"
)

func zstdDecompress(src []byte) ([]byte, error) {
	return zstd.Decompress(nil, src)
}

func newZstdWriter(w io.WriteCloser) io.WriteCloser {
	return zstd.NewWriter(w)
}
