// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package common holds common related files
package common

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	ddogstatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/agentless/types"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// InitStatsd initializes the dogstatsd client
func InitStatsd(sc types.ScannerConfig) ddogstatsd.ClientInterface {
	statsdAddr := fmt.Sprintf("%s:%d", sc.DogstatsdHost, sc.DogstatsdPort)
	statsd, err := ddogstatsd.New(statsdAddr)
	if err != nil {
		log.Warnf("could not init dogstatsd client: %s", err)
		return &ddogstatsd.NoOpClient{}
	}
	return statsd
}

// CtxTerminated cancels the context on termination signal
func CtxTerminated() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-ch:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx
}

// TryGetHostname returns the hostname when possible
func TryGetHostname(ctx context.Context) string {
	ctxhostname, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	hostname, err := utils.GetHostnameWithContext(ctxhostname)
	if err != nil {
		return "unknown"
	}
	return hostname
}
