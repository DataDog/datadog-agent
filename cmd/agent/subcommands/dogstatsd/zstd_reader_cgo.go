// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

//go:build cgo

package dogstatsd

import (
	"io"
	"strings"

	"github.com/DataDog/zstd"
)

func maybeDecompressZstd(r io.Reader, path string) io.Reader {
	if strings.HasSuffix(path, ".zstd") {
		return zstd.NewReader(r)
	}
	return r
}
