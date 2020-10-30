// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package check

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/stretchr/testify/assert"
)

func TestIdentify(t *testing.T) {
	testCheck := &StubCheck{}

	instance1 := integration.Data("key1:value1\nkey2:value2")
	initConfig1 := integration.Data("key:value")
	instance2 := instance1
	initConfig2 := initConfig1
	assert.Equal(t, Identify(testCheck, instance1, initConfig1), Identify(testCheck, instance2, initConfig2))

	instance3 := integration.Data("key1:value1\nkey2:value3")
	initConfig3 := integration.Data("key:value")
	assert.NotEqual(t, Identify(testCheck, instance1, initConfig1), Identify(testCheck, instance3, initConfig3))
}

func TestIDToCheckName(t *testing.T) {
	testCases := []struct {
		in  string
		out string
	}{
		{
			in:  "valid:9505c316b4e4a028",
			out: "valid",
		},
		{
			in:  "",
			out: "",
		},
		{
			in:  "nocolon",
			out: "nocolon",
		},
		{
			in:  "nohash:",
			out: "nohash",
		},
		{
			in:  "multiple:colon:9505c316b4e4a028",
			out: "multiple",
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("case %d: %s", i, tc.in), func(t *testing.T) {
			assert.Equal(t, tc.out, IDToCheckName(ID(tc.in)))
		})
	}
}
