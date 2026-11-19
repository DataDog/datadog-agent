// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build !linux

// Package sockdiag lists TCP listening sockets with a netlink sock_diag dump,
// for use as the source passed to portlist.NewTCPPoller. It is only
// implemented on Linux.
package sockdiag

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/util/port/portlist"
)

// TCPListeners always fails outside Linux.
func TCPListeners() ([]portlist.TCPListener, error) {
	return nil, errors.New("sock_diag is only supported on Linux")
}
