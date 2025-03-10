// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cshared

import "C"

import (
	"context"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

//export pkgUtilHostnameGet
func pkgUtilHostnameGet(unsafeCtxArg unsafe.Pointer, unsafeHostnameRetPtr unsafe.Pointer, unsafeErrRetPtr unsafe.Pointer) {
	ctx := *(*context.Context)(unsafeCtxArg)

	hostnameRetPtr := (*string)(unsafeHostnameRetPtr)
	errRetPtr := (*error)(unsafeErrRetPtr)

	hostname, err := hostname.Get(ctx)
	*hostnameRetPtr = hostname
	*errRetPtr = err
}
