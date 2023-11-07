// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"context"
	"testing"
)

// InfraProvider is an interface that provides methods for managing testing infrastructure
type InfraProvider[Env any] interface {
	ProvisionInfraAndInitializeEnv(ctx context.Context, t *testing.T, name string, failOnMissing bool) (*Env, error)
	DeleteInfra(ctx context.Context, name string) error
}
