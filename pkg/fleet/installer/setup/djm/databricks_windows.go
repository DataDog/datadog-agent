// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package djm contains data-jobs-monitoring installation logic
package djm

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
)

// SetupDatabricks is a not supported on windows
func SetupDatabricks(_ context.Context, _ *env.Env) error {
	return errors.New("djm is not supported on windows")
}
