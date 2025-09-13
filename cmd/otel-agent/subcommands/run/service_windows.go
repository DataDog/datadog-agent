// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && otlp

package run

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/servicemain"
)

// service implements servicemain.Service for otel-agent
type service struct {
	servicemain.DefaultSettings
	cliParams *cliParams
}

func (s *service) Name() string { return "datadog-otel-agent" }

// Init does not need additional setup since args were parsed via Cobra.
func (s *service) Init() error { return nil }

func (s *service) Run(ctx context.Context) error {
	params := s.cliParams
	if params == nil {
		params = &cliParams{GlobalParams: &subcommands.GlobalParams{}}
	}
	// Fallback defaults only if necessary path is not provided
	if len(params.ConfPaths) == 0 || params.CoreConfPath == "" {
		pd, _ := winutil.GetProgramDataDir()
		ddRoot := strings.TrimRight(pd, "\\/")
		if !strings.EqualFold(filepath.Base(ddRoot), "Datadog") {
			ddRoot = filepath.Join(ddRoot, "Datadog")
		}
		if len(params.ConfPaths) == 0 {
			cfg := filepath.Join(ddRoot, "otel-config.yaml")
			params.ConfPaths = []string{"file:" + filepath.ToSlash(cfg)}
		}
		if params.CoreConfPath == "" {
			params.CoreConfPath = filepath.Join(ddRoot, "datadog.yaml")
		}
	}
	return runOTelAgentCommand(ctx, params)
}
