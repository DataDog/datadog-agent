// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	_ "embed"
	"sort"
	"testing"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/assert"
)

//go:embed fixtures/checkrun_bytes
var checkRunData []byte

func TestCheckRun(t *testing.T) {
	t.Run("parseCheckRunPayload empty JSON object should be ignored", func(t *testing.T) {
		checks, err := ParseCheckRunPayload(api.Payload{Data: []byte("{}"), Encoding: encodingEmpty})
		assert.NoError(t, err)
		assert.Empty(t, checks)
	})

	t.Run("parseCheckRunPayload should ignore single check run (non array object)", func(t *testing.T) {
		checks, err := ParseCheckRunPayload(api.Payload{Data: []byte("{\"check\": \"test\", \"status\": 0}"), Encoding: encodingEmpty})
		assert.NoError(t, err)
		assert.Empty(t, checks)
	})

	t.Run("parseCheckRunPayload should return valid checks on valid ", func(t *testing.T) {
		checks, err := ParseCheckRunPayload(api.Payload{Data: checkRunData, Encoding: encodingDeflate})
		assert.NoError(t, err)
		assert.Equal(t, 12, len(checks))
		assert.Equal(t, "snmp.can_check", checks[0].name())
		expectedTags := []string{"agent_host:COMP-N52P6N99MH", "device_namespace:COMP-N52P6N99MH", "snmp_device:192.168.0.3", "snmp_host:41ba948911b9", "snmp_profile:generic-router"}
		sort.Strings(expectedTags)
		gotTags := checks[0].GetTags()
		sort.Strings(gotTags)
		assert.Equal(t, expectedTags, gotTags)
	})
}
