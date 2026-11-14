// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package phonehome

import (
	"path/filepath"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Match dd-procmgrd's Unix default in pkg/procmgr/rust/client/src/lib.rs.
const defaultSupervisorSocket = "/var/run/datadog-procmgrd/dd-procmgrd.sock"

func validSupervisorSocket(path string) bool { return filepath.IsAbs(path) }

func dialSupervisor(path string) (*grpc.ClientConn, error) {
	return grpc.NewClient("unix://"+path, grpc.WithTransportCredentials(insecure.NewCredentials()))
}
