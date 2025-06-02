// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux

package service

import (
	"go.uber.org/fx"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func NewCollector() (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{}, nil
}

func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}
