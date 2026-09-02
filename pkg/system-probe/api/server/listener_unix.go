// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build unix

package server

import (
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewListener creates a Unix Domain Socket Listener
func NewListener(socketAddr string) (net.Listener, error) {
	if len(socketAddr) == 0 {
		return nil, errors.New("uds: empty socket path provided")
	}

	conn, err := filesystem.ListenUnix(socketAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to create system-probe socket: %w", err)
	}

	if err := os.Chmod(socketAddr, 0720); err != nil {
		return nil, fmt.Errorf("socket chmod write-only: %s", err)
	}

	perms, err := filesystem.NewPermission()
	if err != nil {
		return nil, err
	}

	if err := perms.RestrictAccessToUser(socketAddr); err != nil {
		return nil, err
	}

	log.Debugf("uds: %s successfully initialized", conn.Addr())
	return conn, nil
}
