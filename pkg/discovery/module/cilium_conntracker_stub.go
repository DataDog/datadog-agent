// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !linux_bpf

package module

import (
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
)

// ciliumConntracker is a stub for non-linux_bpf builds.
type ciliumConntracker struct{}

// newCiliumConntracker returns nil on non-linux_bpf builds.
func newCiliumConntracker() (*ciliumConntracker, error) {
	return nil, nil
}

// getConnections returns an empty slice on non-linux_bpf builds.
func (cc *ciliumConntracker) getConnections() ([]model.Connection, error) {
	return nil, nil
}

// Close is a no-op on non-linux_bpf builds.
func (cc *ciliumConntracker) Close() error {
	return nil
}
