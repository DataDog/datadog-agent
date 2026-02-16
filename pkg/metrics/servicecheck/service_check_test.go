// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicecheck

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServiceCheckStatusString(t *testing.T) {
	tests := []struct {
		status   ServiceCheckStatus
		expected string
	}{
		{ServiceCheckOK, "OK"},
		{ServiceCheckWarning, "WARNING"},
		{ServiceCheckCritical, "CRITICAL"},
		{ServiceCheckUnknown, "UNKNOWN"},
		{ServiceCheckStatus(99), ""},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.expected, tc.status.String())
	}
}

func TestServiceCheckString(t *testing.T) {
	sc := ServiceCheck{
		CheckName: "my.check",
		Host:      "myhost",
		Ts:        1234567890,
		Status:    ServiceCheckOK,
		Message:   "all good",
		Tags:      []string{"env:prod"},
	}
	s := sc.String()
	assert.Contains(t, s, `"check":"my.check"`)
	assert.Contains(t, s, `"host_name":"myhost"`)
	assert.Contains(t, s, `"status":0`)
}

func TestMarshalStrings(t *testing.T) {
	checks := ServiceChecks{
		{CheckName: "beta.check", Host: "host1", Ts: 200, Status: ServiceCheckWarning, Message: "warn", Tags: []string{"a"}},
		{CheckName: "alpha.check", Host: "host2", Ts: 100, Status: ServiceCheckOK, Message: "ok", Tags: []string{"b"}},
	}

	headers, payload := checks.MarshalStrings()
	assert.Equal(t, []string{"Check", "Hostname", "Timestamp", "Status", "Message", "Tags"}, headers)
	assert.Len(t, payload, 2)
	// should be sorted by check name
	assert.Equal(t, "alpha.check", payload[0][0])
	assert.Equal(t, "beta.check", payload[1][0])
}

func TestMarshalStringsSameNameSortByTimestamp(t *testing.T) {
	checks := ServiceChecks{
		{CheckName: "my.check", Host: "h", Ts: 300, Status: ServiceCheckOK, Tags: []string{}},
		{CheckName: "my.check", Host: "h", Ts: 100, Status: ServiceCheckOK, Tags: []string{}},
	}
	_, payload := checks.MarshalStrings()
	assert.Equal(t, "100", payload[0][2])
	assert.Equal(t, "300", payload[1][2])
}

func TestMarshalStringsEmpty(t *testing.T) {
	checks := ServiceChecks{}
	headers, payload := checks.MarshalStrings()
	assert.NotNil(t, headers)
	assert.Empty(t, payload)
}
