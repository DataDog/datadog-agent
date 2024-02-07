// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build !linux

// Package nbd defines the Network Block Device and provides the functionality
// to start and stop the NBD server and client.
package nbd

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/types"

	"github.com/aws/aws-sdk-go-v2/service/ebs"
)

// StartNBDBlockDevice starts the NBD server and client for the given device
// name with the provided backend.
func StartNBDBlockDevice(_ *types.ScanTask, _ string, _ backend.Backend) error {
	return fmt.Errorf("ebsblockdevice: not supported on this platform")
}

// StopNBDBlockDevice stops the NBD server and client for the given device name.
func StopNBDBlockDevice(_ context.Context, _ string) {
}
