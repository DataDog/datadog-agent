// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package etwimpl has no implementation on non-Windows platforms
package etwimpl

// import "C" fixes the error C source files not allowed when not using cgo or SWIG: session.c (typecheck)
import "C"
import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
var Module = fxutil.Component()
