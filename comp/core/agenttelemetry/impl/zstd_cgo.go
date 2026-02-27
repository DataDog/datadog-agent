// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build cgo

package agenttelemetryimpl

import "github.com/DataDog/zstd"

func zstdCompressLevel(src []byte, level int) ([]byte, error) {
	return zstd.CompressLevel(nil, src, level)
}
