// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ec2

package ec2

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// GetAccountID returns the account ID of the current AWS instance
func GetAccountID(ctx context.Context) (string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}

	ec2id, err := GetInstanceIdentity(ctx)
	if err != nil {
		return "", err
	}

	return ec2id.AccountID, nil
}
