// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpftest

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// RequireKernelVersion skips a test if the minimum kernel version is not met
func RequireKernelVersion(tb testing.TB, version kernel.Version) {
	if kv < version {
		tb.Skipf("skipping test; it requires kernel version %s or later, running on: %s", version, kv)
	}
}
