// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !ec2

package ec2

import (
	"context"
	"fmt"
)

// GetAccountID returns the account ID of the current AWS instance
func GetAccountID(_ context.Context) (string, error) {
	return "", fmt.Errorf("ec2 is disabled in the binary")
}
