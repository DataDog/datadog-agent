// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !functionaltests

// Package hostnameutils holds utils/hostname related files
package hostnameutils

import (
	"context"
	"time"

	"github.com/avast/retry-go/v4"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// getHostnameFromAgent attempts to acquire a hostname by connecting to the
// core agent's gRPC endpoints extending the given context.
func getHostnameFromAgent(ctx context.Context, ipcComp ipc.Component) (string, error) {
	var hostname string
	err := retry.Do(func() error {
		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
		if err != nil {
			return err
		}

		client, err := grpc.GetDDAgentClient(ctx, ipcAddress, pkgconfigsetup.GetIPCPort(), ipcComp.GetTLSClientConfig())
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
