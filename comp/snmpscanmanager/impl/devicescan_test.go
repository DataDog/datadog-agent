// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package snmpscanmanagerimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_deviceScan_isSuccess(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		deviceScan     deviceScan
		expectedResult bool
	}{
		{
			name: "is not success",
			deviceScan: deviceScan{
				DeviceIP:   "10.0.0.1",
				ScanStatus: failedStatus,
				ScanEndTs:  now,
			},
			expectedResult: false,
		},
		{
			name: "is success",
			deviceScan: deviceScan{
				DeviceIP:   "10.0.0.1",
				ScanStatus: successStatus,
				ScanEndTs:  now,
			},
			expectedResult: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			isSuccess := test.deviceScan.isSuccess()
			assert.Equal(t, test.expectedResult, isSuccess)
		})
	}
}

func Test_ipSet_add(t *testing.T) {
	set := ipSet{}

	set.add("10.0.0.1")
	assert.Len(t, set, 1)
	assert.Contains(t, set, "10.0.0.1")

	set.add("10.0.0.2")
	assert.Len(t, set, 2)
	assert.Contains(t, set, "10.0.0.2")
}

func Test_ipSet_contains(t *testing.T) {
	set := ipSet{}

	set.add("10.0.0.1")
	set.add("10.0.0.2")

	assert.True(t, set.contains("10.0.0.1"))
	assert.True(t, set.contains("10.0.0.2"))
	assert.False(t, set.contains("10.0.0.3"))
	assert.False(t, set.contains("10.0.0.4"))
}
