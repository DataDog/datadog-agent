// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux

package sack

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
)

var errPlatformNotSupported = &NotSupportedError{
	Err: errors.New("SACK traceroute is not supported on this platform"),
}

// RunSackTraceroute is not supported
func RunSackTraceroute(_ctx context.Context, _p Params) (*common.Results, error) {
	return nil, errPlatformNotSupported
}
