// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

// Package hostnameutils holds utils/hostname related files
package hostnameutils

import (
	"context"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
)

// getHostnameFromAgent returns a fake hostname for functional tests.
func getHostnameFromAgent(ctx context.Context, _ ipc.Component) (string, error) {
	return "functional_tests_host", nil
}
