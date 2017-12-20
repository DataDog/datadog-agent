// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build !kubeapiserver

package apiserver

import (
	"errors"
)

var (
	// ErrNotCompiled is returned if kubernetes apiserver support is not compiled in.
	// User classes should handle that case as gracefully as possible.
	ErrNotCompiled = errors.New("kubernetes apiserver support not compiled in")
)
