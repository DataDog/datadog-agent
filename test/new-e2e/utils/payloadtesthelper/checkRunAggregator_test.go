// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package payloadtesthelper

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/assert"
)

//go:embed fixtures/api_v1_check_run.txt
var checkRunBody []byte

func TestCheckRun(t *testing.T) {
	t.Run("UnmarshallPayloads", func(t *testing.T) {
		agg := NewCheckRunAggregator()
		err := agg.UnmarshallPayloads(checkRunBody)
		assert.NoError(t, err)
		assert.Equal(t, 4, len(agg.checkRunByName))
	})

	t.Run("ContainsCheckName", func(t *testing.T) {
		agg := NewCheckRunAggregator()
		err := agg.UnmarshallPayloads(checkRunBody)
		assert.NoError(t, err)
		assert.True(t, agg.ContainsCheckName("datadog.agent.up"))
		assert.False(t, agg.ContainsCheckName("invalid.check.name"))
	})

	t.Run("ContainsCheckNameAndTags", func(t *testing.T) {
		agg := NewCheckRunAggregator()
		err := agg.UnmarshallPayloads(checkRunBody)
		assert.NoError(t, err)
		assert.True(t, agg.ContainsCheckNameAndTags("snmp.can_check", []string{"snmp_device:192.168.0.3", "snmp_host:41ba948911b9"}))
		assert.False(t, agg.ContainsCheckNameAndTags("snmp.can_check", []string{"invalid:tag"}))
	})
}
