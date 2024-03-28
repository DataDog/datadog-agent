// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package listeners

import (
	"errors"
	"net"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// ErrLinuxOnly is emitted on non-linux platforms
var ErrLinuxOnly = errors.New("only implemented on Linux hosts")

// getUDSAncillarySize returns 0 on non-linux hosts
func getUDSAncillarySize() int {
	return 0
}

// enableUDSPassCred returns a "not implemented" error on non-linux hosts
//
//nolint:revive // TODO(AML) Fix revive linter
func enableUDSPassCred(conn *net.UnixConn) error {
	return ErrLinuxOnly
}

// processUDSOrigin returns a "not implemented" error on non-linux hosts
//
//nolint:revive // TODO(AML) Fix revive linter
func processUDSOrigin(oob []byte, wmeta optional.Option[workloadmeta.Component], pidMap pidmap.Component) (int, string, error) {
	return 0, packets.NoOrigin, ErrLinuxOnly
}
