// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package platform

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

func TestWindowsKernelReleaseMatchesSharedVersion(t *testing.T) {
	version, err := winutil.GetWindowsVersionComponents()
	if err != nil {
		t.Fatal(err)
	}

	kernelRelease, err := CollectInfo().KernelRelease.Value()
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("%s.%s.%s", version.Major, version.Minor, version.Build)
	if kernelRelease != want {
		t.Errorf("KernelRelease = %q, want %q", kernelRelease, want)
	}
}
