// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package schema

import (
	"encoding/json"
	"fmt"
	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testcaseExpected struct {
	ErrorPatterns []string `json:"error_patterns"`
}

func Test_DeviceProfileRcConfigJsonSchema(t *testing.T) {
	// language=json
	instanceJson := `{
	"profile_definition": {
		"name": "my-profile"
	}
}`

	err := assertAgainstSchema(t, instanceJson)

	fmt.Printf("%#v\n", err) // using %#v prints errors hierarchy
	require.NoError(t, err)
}

func Test_Schema_TextCases(t *testing.T) {
	var testcases []string
	err := filepath.WalkDir("./schema_testcases", func(s string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if strings.HasSuffix(d.Name(), "_expected.json") {
			return nil
		}
		if filepath.Ext(d.Name()) == ".json" {
			testcases = append(testcases, s)
		}
		return nil
	})
	require.NoError(t, err)

	for _, testcaseJsonPath := range testcases {
		t.Run(testcaseJsonPath, func(t *testing.T) {
			content, err := os.ReadFile(testcaseJsonPath)
			require.NoError(t, err)

			validationErr := assertAgainstSchema(t, string(content))
			validationErrStr := fmt.Sprintf("%#v\n", validationErr) // using %#v prints errors hierarchy

			if validationErr != nil {
				// Print validation error to ease debugging tests
				fmt.Printf("--- ACTUAL VALIDATION ERRORS --------------------------------------------\n")
				fmt.Printf(validationErrStr)
				fmt.Printf("-------------------------------------------------------------------------\n")
			}

			testcaseExpectedErrPath := strings.ReplaceAll(testcaseJsonPath, ".json", "_expected.json")
			testcaseExpectedErr, err := os.ReadFile(testcaseExpectedErrPath)
			require.NoError(t, err)

			var expected testcaseExpected
			err = json.Unmarshal(testcaseExpectedErr, &expected)
			require.NoError(t, err)

			for _, expectedErrorPattern := range expected.ErrorPatterns {
				assert.Regexp(t, expectedErrorPattern, validationErrStr)
			}
			if len(expected.ErrorPatterns) == 0 {
				assert.NoError(t, validationErr)
			}
		})
	}
}

func assertAgainstSchema(t *testing.T, instanceJson string) error {
	sch, err := jsonschema.CompileString("schema.json", string(deviceProfileRcConfigJsonschema))
	require.NoError(t, err)

	var v interface{}
	err = json.Unmarshal([]byte(instanceJson), &v)
	require.NoError(t, err)

	err = sch.Validate(v)
	return err
}
