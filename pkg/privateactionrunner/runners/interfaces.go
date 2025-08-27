// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package runners

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/credentials"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/remoteconfig"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// TaskExecutor defines the interface for executing tasks
type TaskExecutor interface {
	// RunTask executes a single task with the provided credential
	RunTask(ctx context.Context, task *types.Task, credential interface{}) (interface{}, error)

	// GetOpmsClient returns the OPMS client for task operations
	GetOpmsClient() opms.Client

	// GetTaskVerifier returns the task verifier for signature verification
	GetTaskVerifier() *taskverifier.TaskVerifier

	// GetResolver returns the credential resolver
	GetResolver() credentials.PrivateCredentialResolver

	// GetConfig returns the configuration
	GetConfig() *config.Config

	// GetKeysManager returns the keys manager
	GetKeysManager() remoteconfig.KeysManager
}

// LoopConfig contains configuration specific to the task loop
type LoopConfig struct {
	LoopInterval    time.Duration
	RunnerPoolSize  int32
	MinBackoff      time.Duration
	MaxBackoff      time.Duration
	WaitBeforeRetry time.Duration
	MaxAttempts     int32
}

// GetLoopConfig extracts loop-specific configuration from the main config
func GetLoopConfig(cfg *config.Config) *LoopConfig {
	return &LoopConfig{
		LoopInterval:    cfg.LoopInterval,
		RunnerPoolSize:  cfg.RunnerPoolSize,
		MinBackoff:      cfg.MinBackoff,
		MaxBackoff:      cfg.MaxBackoff,
		WaitBeforeRetry: cfg.WaitBeforeRetry,
		MaxAttempts:     cfg.MaxAttempts,
	}
}
