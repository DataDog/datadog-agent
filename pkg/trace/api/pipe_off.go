// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package api

import (
	"errors"
	"net"

	"github.com/DataDog/datadog-go/v5/statsd"
)

// listenPipe return a nil-listener and an error on non-Windows operating systems.
func listenPipe(_, _ string, _, _ int, _ statsd.ClientInterface) (net.Listener, error) {
	return nil, errors.New("Windows named pipes are only supported on Windows operating systems")
}
