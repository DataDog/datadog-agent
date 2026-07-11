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
// os hostname is usable.
//
// NOTE: the trace-agent carries a copy of this logic in
// comp/trace/config/impl/os_hostname_others.go (it cannot import this package directly
// without pulling in heavy transitive dependencies). Keep the two in sync.
func isOSHostnameUsable(_ context.Context) bool {
	return true
}
