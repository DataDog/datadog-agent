// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

// Package agent holds agent related files
package agent

import "context"

type telemetry struct{}

func (t *telemetry) registerProfiledContainer(_, _ string) {}

func (t *telemetry) run(_ context.Context, _ *RuntimeSecurityAgent) {}
