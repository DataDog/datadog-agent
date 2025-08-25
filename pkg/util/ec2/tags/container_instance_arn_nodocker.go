// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ec2 && !docker

package tags

import (
	"context"
	"fmt"
)

// getContainerInstanceARN is a stub used when the `docker` build tag is not enabled.
func getContainerInstanceARN(_ context.Context) (string, error) {
	return "", fmt.Errorf("ECS metadata is not available without docker build tag")
}
