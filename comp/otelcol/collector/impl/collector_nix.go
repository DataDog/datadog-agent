// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && otlp

package collectorimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	corelog "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

// setupShutdown supports stop signals
func setupShutdown(_ context.Context, log corelog.Component, shutdowner compdef.Shutdowner) {
	go func() {
		// Wait for stop signal
		select {
		case <-signals.Stopper:
			log.Info("Received stop command, shutting down...")
		case <-signals.ErrorStopper:
			_ = log.Critical("The OTel Agent has encountered an error, shutting down...")
		}
		_ = shutdowner.Shutdown()
	}()
}
