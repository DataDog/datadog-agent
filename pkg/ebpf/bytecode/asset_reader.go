// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package bytecode contains types and functions for eBPF bytecode
package bytecode

import (
	"io"
)

// AssetReader describes the combination of both io.Reader and io.ReaderAt
type AssetReader interface {
	io.Reader
	io.ReaderAt
	io.Closer
}
