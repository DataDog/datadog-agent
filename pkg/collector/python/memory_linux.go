// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && linux

package python

import (
	// Makes backtraces include Cgo frames. Linux-only due to https://github.com/golang/go/issues/45558
	_ "github.com/ianlancetaylor/cgosymbolizer"
)
