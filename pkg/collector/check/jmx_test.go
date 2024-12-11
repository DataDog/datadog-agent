// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

func TestIsJMXInstance(t *testing.T) {

	var cases = []struct {
		instance      integration.Data
		initConfig    integration.Data
		expectedIsJmx bool
	}{
		{integration.Data("{}"), integration.Data("{}"), false},
		{integration.Data("{}"), integration.Data("{\"is_jmx\": true}"), true},
		{integration.Data("{\"is_jmx\": true}"), integration.Data("{\"is_jmx\": true}"), true},
		{integration.Data("{\"is_jmx\": true}"), integration.Data("{}"), true},
		{integration.Data("{}"), integration.Data("{\"is_jmx\": false}"), false},
		{integration.Data("{\"is_jmx\": false}"), integration.Data("{\"is_jmx\": false}"), false},
		{integration.Data("{\"is_jmx\": false}"), integration.Data("{}"), false},
		{integration.Data("{\"loader\": jmx}"), integration.Data("{}"), true},
		{integration.Data("{\"loader\": python}"), integration.Data("{}"), false},
		{integration.Data("{}"), integration.Data("{\"loader\": jmx}"), true},
		{integration.Data("{\"loader\": python}"), integration.Data("{\"loader\": jmx}"), false},
		{integration.Data("{\"loader\": jmx}"), integration.Data("{\"loader\": python}"), true},
		{integration.Data("{\"loader\": python, \"is_jmx\": true}"), integration.Data("{}"), false},
		{integration.Data("{}"), integration.Data("{\"loader\": python, \"is_jmx\": true}"), false},
		{integration.Data("{\"loader\": jmx}"), integration.Data("{\"loader\": python, \"is_jmx\": false}"), true},
	}

	for _, tc := range cases {
		isJmx := IsJMXInstance("name", tc.instance, tc.initConfig)
		assert.Equal(t, tc.expectedIsJmx, isJmx)
	}
}
