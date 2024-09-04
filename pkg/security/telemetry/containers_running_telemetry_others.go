// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package telemetry

import (
	"context"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-go/v5/statsd"
)

type ContainersRunningTelemetry struct{}

func NewContainersRunningTelemetry(_ *config.RuntimeSecurityConfig, _ statsd.ClientInterface, _ workloadmeta.Component, _ bool) (*ContainersRunningTelemetry, error) {
	return nil, nil
}

func (t *ContainersRunningTelemetry) Run(_ context.Context) {}
