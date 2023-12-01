//go:build !linux

package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/ebs"
)

type EBSBlockDeviceOptions struct {
	EBSClient   *ebs.Client
	Name        string
	DeviceName  string
	Description string
	SnapshotARN arn.ARN
}

func SetupEBSBlockDevice(ctx context.Context, opts EBSBlockDeviceOptions) error {
	return fmt.Errorf("ebsblockdevice: not supported on this platform")
}
