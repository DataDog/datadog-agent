// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

package systemd

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

func (*Client) ListSystemServices(context.Context, builtins.SystemServiceListRequest) ([]builtins.SystemServiceState, error) {
	return nil, managerUnsupported()
}

func (*Client) InspectSystemServices(context.Context, []string) ([]builtins.SystemServiceState, error) {
	return nil, managerUnsupported()
}

func (*Client) RunSystemServiceJobs(context.Context, builtins.SystemServiceJobAction, []string) error {
	return managerUnsupported()
}

func (*Client) EnableSystemServices(context.Context, []string) error {
	return managerUnsupported()
}

func (*Client) DisableSystemServices(context.Context, []string) error {
	return managerUnsupported()
}

func managerUnsupported() error {
	return fmt.Errorf("%w: systemd manager access requires Linux", builtins.ErrSystemdUnsupported)
}
