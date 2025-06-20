// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostnameutils holds utils/hostname related files
package hostnameutils

import (
	"context"
	"sync"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
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
func GetHostname(ipcComp ipc.Component) (string, error) {
	hostnameLock.RLock()
	if cachedHostname != "" {
		hostnameLock.RUnlock()
		return cachedHostname, nil
	}
	hostnameLock.RUnlock()

	hostname, err := getHostnameFromAgent(context.Background(), ipcComp)

	if hostname != "" {
		hostnameLock.Lock()
		cachedHostname = hostname
		hostnameLock.Unlock()
	}

	return hostname, err
}

// GetHostnameWithContextAndFallback attempts to acquire a hostname by connecting to the
// core agent's gRPC endpoints extending the given context, or falls back to local resolution
func GetHostnameWithContextAndFallback(ctx context.Context, ipcComp ipc.Component) (string, error) {
	hostnameDetected, err := getHostnameFromAgent(ctx, ipcComp)
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
