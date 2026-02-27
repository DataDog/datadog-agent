// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package hostnameimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// isOSHostnameUsable returns false if the agent is containerized on Windows.
func isOSHostnameUsable(_ context.Context, _ pkgconfigmodel.Reader) bool {
	return !env.IsContainerized()
}
