// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostname

import (
	"context"
)

// isHostnameCanonicalForIntake returns true if the intake will use the hostname as canonical hostname.
func isHostnameCanonicalForIntake(ctx context.Context, hostname string) bool {
	panic("not called")
}

func warnIfNotCanonicalHostname(ctx context.Context, hostname string) {
	panic("not called")
}

func warnAboutFQDN(ctx context.Context, hostname string) {
	panic("not called")
}
