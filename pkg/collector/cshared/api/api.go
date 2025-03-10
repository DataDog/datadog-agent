// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cshared

// extern void pkgUtilHostnameGet(void *unsafeCtxArg, void *hostnameRetPtr, void *errRetPtr);
import "C"

import (
	"context"
	"unsafe"
)

// GetHostname returns the hostname used by the agent
func GetHostname(ctx context.Context) (string, error) {
	var hostname string
	var err error

	C.pkgUtilHostnameGet(unsafe.Pointer(&ctx), unsafe.Pointer(&hostname), unsafe.Pointer(&err))

	return hostname, err
}
