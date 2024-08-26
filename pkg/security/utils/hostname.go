// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils holds utils related files
package utils

import (
	"context"
	"sync"
	"time"

	"github.com/avast/retry-go/v4"

	"github.com/DataDog/datadog-agent/pkg/config"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// maxAttempts is the maximum number of times we try to get the hostname
	// from the core-agent before bailing out.
	maxAttempts = 6
)

var (
	hostnameLock   sync.RWMutex
	cachedHostname string
)

// GetHostname attempts to acquire a hostname by connecting to the core
// agent's gRPC endpoints.
func GetHostname() (string, error) {
	hostnameLock.RLock()
	if cachedHostname != "" {
		hostnameLock.RUnlock()
		return cachedHostname, nil
	}
	hostnameLock.RUnlock()

	hostname, err := getHostnameFromAgent(context.Background())

	if hostname != "" {
		hostnameLock.Lock()
		cachedHostname = hostname
		hostnameLock.Unlock()
	}

	return hostname, err
}

// getHostnameFromAgent attempts to acquire a hostname by connecting to the
// core agent's gRPC endpoints extending the given context.
func getHostnameFromAgent(ctx context.Context) (string, error) {
	var hostname string
	err := retry.Do(func() error {
		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		ipcAddress, err := config.GetIPCAddress()
		if err != nil {
			return err
		}

		client, err := grpc.GetDDAgentClient(ctx, ipcAddress, config.GetIPCPort())
		if err != nil {
			return err
		}

		reply, err := client.GetHostname(ctx, &pbgo.HostnameRequest{})
		if err != nil {
			return err
		}

		log.Debugf("Acquired hostname from gRPC: %s", reply.Hostname)

		hostname = reply.Hostname
		return nil
	}, retry.LastErrorOnly(true), retry.Attempts(maxAttempts), retry.Context(ctx))
	return hostname, err
}

// GetHostnameWithContextAndFallback attempts to acquire a hostname by connecting to the
// core agent's gRPC endpoints extending the given context, or falls back to local resolution
func GetHostnameWithContextAndFallback(ctx context.Context) (string, error) {
	hostnameDetected, err := getHostnameFromAgent(ctx)
	if err != nil {
		log.Warnf("Could not resolve hostname from core-agent: %v", err)
		hostnameDetected, err = hostname.Get(ctx)
		if err != nil {
			return "", err
		}
	}
	log.Infof("Hostname is: %s", hostnameDetected)
	return hostnameDetected, nil
}
