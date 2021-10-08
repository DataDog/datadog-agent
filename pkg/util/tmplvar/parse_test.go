// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tmplvar

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseTemplateVar(t *testing.T) {
	testCases := []struct {
		tmpl, name, key string
	}{
		{
			"%%host%%",
			"host",
			"",
		},
		{
			"%%host_0%%",
			"host",
			"0",
		},
		{
			"%%host 0%%",
			"host0",
			"",
		},
		{
			"%%host_0_1%%",
			"host",
			"0_1",
		},
		{
			"%%host_network_name%%",
			"host",
			"network_name",
		},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("#%d", i), func(t *testing.T) {
			name, key := parseTemplateVar([]byte(testCase.tmpl))
			assert.Equal(t, testCase.name, string(name))
			assert.Equal(t, testCase.key, string(key))
		})
	}
}
