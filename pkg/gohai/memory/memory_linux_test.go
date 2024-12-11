// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package memory

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseMemoryInfo checks that parseMemoryInfo correctly reads the format from /proc/meminfo.
// This format is described in the redhat doc:
// https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/6/html/deployment_guide/s2-proc-meminfo
func TestParseMemoryInfo(t *testing.T) {
	meminfo := `MemTotal:        1921988 kB
MemFree:       1374408 kB
SwapTotal:       1048572 kB
AnonHugePages:         0 kB
HugePages_Total:       0`
	reader := strings.NewReader(meminfo)

	totalBytes, swapTotalKb, err := parseMemoryInfo(reader)
	require.NoError(t, err)

	totalBytesVal, err := totalBytes.Value()
	require.NoError(t, err)
	require.EqualValues(t, 1921988*1024, totalBytesVal)

	swapTotalKbVal, err := swapTotalKb.Value()
	require.NoError(t, err)
	require.EqualValues(t, 1048572, swapTotalKbVal)
}

func TestParseMemoryInfoWeird(t *testing.T) {
	meminfo := `	MemTotal 	: 	 	 1921988 kB

HugePages_Total:       0`
	reader := strings.NewReader(meminfo)

	totalBytes, swapTotalKb, err := parseMemoryInfo(reader)
	require.NoError(t, err)

	totalBytesVal, err := totalBytes.Value()
	require.NoError(t, err)
	require.EqualValues(t, 1921988*1024, totalBytesVal)

	_, err = swapTotalKb.Value()
	require.ErrorContains(t, err, "not found")
}
