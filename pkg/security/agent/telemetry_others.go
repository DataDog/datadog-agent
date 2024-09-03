// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

// Package agent holds agent related files
package agent

import "context"

type telemetry struct{}

func (t *telemetry) run(_ context.Context) {}

type profContainersTelemetry struct{}

func (t *profContainersTelemetry) registerProfiledContainer(_, _ string) {}

func (t *profContainersTelemetry) run(_ context.Context) {}
