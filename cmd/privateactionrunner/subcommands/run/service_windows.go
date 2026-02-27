// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package run

import (
	"context"
	"errors"
	"path/filepath"

	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/servicemain"
)

const (
	ServiceName = "datadog-agent-action"
)

var (
	defaultConfPath = filepath.Join(defaultpaths.ConfPath, "datadog.yaml")
)

type windowsService struct {
	servicemain.DefaultSettings
}

func NewService() servicemain.Service {
	return &windowsService{}
}

func (s *windowsService) Name() string {
	return ServiceName
}

func (s *windowsService) Init() error {
	return nil
}

func (s *windowsService) Run(ctx context.Context) error {
	err := runPrivateActionRunner(ctx, defaultConfPath, nil)
	if errors.Is(err, privateactionrunner.ErrNotEnabled) {
		return servicemain.ErrCleanStopAfterInit
	}
	return err
}
