// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2

import (
	"context"
	"fmt"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	ec2internal "github.com/DataDog/datadog-agent/pkg/util/ec2/internal"
)

// GetAccountID returns the account ID of the current AWS instance
func GetAccountID(ctx context.Context) (string, error) {
	if !pkgconfigsetup.IsCloudProviderEnabled(CloudProviderName, pkgconfigsetup.Datadog()) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}

	ec2id, err := GetInstanceIdentity(ctx)
	if err != nil {
		return "", err
	}

	return ec2id.AccountID, nil
}

// EC2Identity holds the instances identity document
// nolint: revive
type EC2Identity = ec2internal.EC2Identity

// GetInstanceIdentity returns the instance identity document for the current instance
func GetInstanceIdentity(ctx context.Context) (*EC2Identity, error) {
	return ec2internal.GetInstanceIdentity(ctx)
}
