// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/servicemain"
)

type service struct {
	servicemain.DefaultSettings
	cliParams       *Params
	defaultConfPath string
}

func (s *service) Name() string {
	return config.ServiceName
}

func (s *service) Init() error {
	// Nothing to do, kept empty for compatibility with previous implementation.
	return nil
}

func (s *service) Run(ctx context.Context) error {
	return runTraceAgentProcess(ctx, s.cliParams, s.defaultConfPath)
}
