// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && !linux

package hostname

import (
	"context"
)

// On non-Linux, non-Windows, we don't support containers and will assume
// os hostname is usable
func isOSHostnameUsable(ctx context.Context) bool {
	return true
}
