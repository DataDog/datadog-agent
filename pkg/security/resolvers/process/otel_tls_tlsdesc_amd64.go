// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && amd64

package process

import "errors"

// tlsdescReturnsArg is only consulted on aarch64 (variant I). On amd64 the
// sign of the TLSDESC argument is enough, so this is unreachable.
func tlsdescReturnsArg(otelMemReader, uint64) (bool, error) {
	return false, errors.New("internal error: tlsdescReturnsArg called on amd64")
}
