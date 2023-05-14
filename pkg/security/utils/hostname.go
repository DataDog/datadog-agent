// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetHostname attempts to acquire a hostname by connecting to the core agent's
// gRPC endpoints
func GetHostname() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client, err := grpc.GetDDAgentClient(ctx)
	if err != nil {
		return "", err
	}

	reply, err := client.GetHostname(ctx, &pbgo.HostnameRequest{})
	if err != nil {
		return "", err
	}

	log.Debugf("Acquired hostname from gRPC: %s", reply.Hostname)
	return reply.Hostname, nil
}
