// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package state

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseFilePath(t *testing.T) {
	tests := []struct {
		input  string
		err    bool
		output configPath
	}{
		{
			input:  "datadog/2/APM_SAMPLING/fc18c18f-939a-4017-b428-af03678f6c1a/file1",
			err:    false,
			output: configPath{Source: sourceDatadog, OrgID: 2, Product: "APM_SAMPLING", ConfigID: "fc18c18f-939a-4017-b428-af03678f6c1a", Name: "file1"},
		},
		{
			input:  "employee/APM_SAMPLING/fc18c18f-939a-4017-b428-af03678f6c1a/file1",
			err:    false,
			output: configPath{Source: sourceEmployee, Product: "APM_SAMPLING", ConfigID: "fc18c18f-939a-4017-b428-af03678f6c1a", Name: "file1"},
		},
		{
			input: "user/5343/TESTING1/static_id/f3045934w_dogfile",
			err:   true,
		},
		{
			input: "user/a/TESTING1/static_id/f3045934w_dogfile",
			err:   true,
		},
		{
			input: "/5343/TESTING1/static_id/f3045934w_dogfile",
			err:   true,
		},
		{
			input: "user//TESTING1/static_id/f3045934w_dogfile",
			err:   true,
		},
		{
			input: "user/5343//static_id/f3045934w_dogfile",
			err:   true,
		},
		{
			input: "user/5343/TESTING1//f3045934w_dogfile",
			err:   true,
		},
		{
			input: "user/5343/TESTING1/static_id/",
			err:   true,
		},
	}
	for _, test := range tests {
		t.Run(test.input, func(tt *testing.T) {
			output, err := parseConfigPath(test.input)
			if test.err {
				assert.Error(tt, err)
			} else {
				assert.Equal(tt, test.output, output)
				assert.NoError(tt, err)
			}
		})
	}
}
