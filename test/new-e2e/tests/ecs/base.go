// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package ecs

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// BaseSuite is the base test suite for container tests, providing common functionality
// for ECS, Docker, and other container platform tests.
type BaseSuite[Env any] struct {
	e2e.BaseSuite[Env]

	Fakeintake  *fakeintake.Client
	ClusterName string
}
