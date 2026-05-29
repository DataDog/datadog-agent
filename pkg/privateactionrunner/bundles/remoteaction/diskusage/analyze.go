//go:build !windows

package com_datadoghq_remoteaction_diskusage

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
)

type AnalyzeHandler struct{}

func NewAnalyzeHandler() *AnalyzeHandler {
	return &AnalyzeHandler{}
}

func (h *AnalyzeHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	return nil, errors.New("com.datadoghq.remoteaction.diskusage.analyze is not available on this platform/build")
}
