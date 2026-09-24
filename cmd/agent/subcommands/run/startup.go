// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package run

import (
	"context"

	"github.com/DataDog/datadog-agent/cmd/agent/subcommands/run/internal/lite"
)

type startupState struct {
	params  lite.Params
	started bool
}

func (s *startupState) finish(err error) error {
	if err != nil && !s.started {
		// Startup may already have canceled Fx's context. Reporting has its own
		// bounded lifetime and never replaces or logs the original error.
		_ = lite.Rescue(context.Background(), s.params, err)
	}
	return err
}
