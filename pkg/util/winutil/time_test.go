// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package winutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFileTimeToUnix(t *testing.T) {

	testcases := []struct {
		filetime uint64
		seconds  uint64
		nano     uint64
	}{
		// 1970/01/01 00:00:00
		{116444736000000000, 0, 0},
		// 2023/04/06 19:34:17
		{133252832570000000, 1680809657, 1680809657000000000},
		// 2023/04/06 19:28:16
		{133252828968413292, 1680809296, 1680809296841329200},
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.nano, FileTimeToUnixNano(tc.filetime), "Should correctly convert filetime %d to nanoseconds", tc.filetime)
		assert.Equal(t, tc.seconds, FileTimeToUnix(tc.filetime), "Should correctly convert filetime %d to seconds", tc.filetime)
	}
}
