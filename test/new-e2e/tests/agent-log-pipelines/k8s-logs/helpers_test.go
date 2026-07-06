// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package k8sfiletailing

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

func fakeintakeRouteStats(fi *components.FakeIntake) string {
	stats, err := fi.Client().RouteStats()
	if err != nil {
		return fmt.Sprintf("RouteStats error: %v", err)
	}
	return fmt.Sprintf("RouteStats: %v", stats)
}
