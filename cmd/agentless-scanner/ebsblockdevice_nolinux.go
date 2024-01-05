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

func startEBSBlockDevice(_ string, _ *ebs.Client, _ string, _ arn.ARN) error {
	return fmt.Errorf("ebsblockdevice: not supported on this platform")
}

func stopEBSBlockDevice(_ context.Context, _ string) {
}
