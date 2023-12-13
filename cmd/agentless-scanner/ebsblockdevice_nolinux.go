// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build !linux

package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/ebs"
)

// EBSBlockDeviceOptions represents the options of the EBS block device.
type EBSBlockDeviceOptions struct {
	EBSClient   *ebs.Client
	DeviceName  string
	SnapshotARN arn.ARN
	RunClient   bool
}

// EBSBlockDevice is used to create an EBS block device using NBD.
type EBSBlockDevice struct {
	EBSBlockDeviceOptions
	wg sync.WaitGroup
}

// NewEBSBlockDevice sets up the EBS block device.
func NewEBSBlockDevice(_ EBSBlockDeviceOptions) error {
	return fmt.Errorf("ebsblockdevice: not supported on this platform")
}

// Start runs the NBD server and client if required.
func (bd *EBSBlockDevice) Start(_ context.Context) error {
	return nil
}

// WaitCleanup waits after context has been canceled for a complete cleanup of
// the running NBD server and client.
func (bd *EBSBlockDevice) WaitCleanup() {
}
