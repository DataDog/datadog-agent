// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !sharedlibrarycheck

// Package ffi handles shared libraries through cgo.
package ffi

/*
#cgo CFLAGS: -I "${SRCDIR}/../../../../rtloader/include"
*/
import "C"
