// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostname provides utilities to detect the hostname of the host.
package hostname

import (
	"context"
	"os"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/azure"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/gce"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
)

// for testing purposes
var (
	isFargateInstance = fargate.IsFargateInstance
	ec2GetInstanceID  = ec2.GetInstanceID
	isContainerized   = config.IsContainerized //nolint:unused
	gceGetHostname    = gce.GetHostname
	azureGetHostname  = azure.GetHostname
	osHostname        = os.Hostname
	fqdnHostname      = getSystemFQDN
	osHostnameUsable  = isOSHostnameUsable
)

// Data contains hostname and the hostname provider
type Data struct {
	Hostname string
	Provider string
}

func fromConfig(ctx context.Context, _ string) (string, error) {
	panic("not called")
}

func fromHostnameFile(ctx context.Context, _ string) (string, error) {
	panic("not called")
}

func fromFargate(_ context.Context, _ string) (string, error) {
	panic("not called")
}

func fromGCE(ctx context.Context, _ string) (string, error) {
	panic("not called")
}

func fromAzure(ctx context.Context, _ string) (string, error) {
	panic("not called")
}

func fromFQDN(ctx context.Context, _ string) (string, error) {
	panic("not called")
}

func fromOS(ctx context.Context, currentHostname string) (string, error) {
	panic("not called")
}

func getValidEC2Hostname(ctx context.Context) (string, error) {
	panic("not called")
}

func fromEC2(ctx context.Context, currentHostname string) (string, error) {
	panic("not called")
}
