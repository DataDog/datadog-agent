// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/status"

	"github.com/stretchr/testify/require"
)

func TestRender(t *testing.T) {
	// We're checking that some dates are correctly formatted in the HTML
	// so we need to set the timezone to UTC to avoid issues.
	originalTZ := os.Getenv("TZ")
	os.Setenv("TZ", "UTC")
	defer func() {
		os.Setenv("TZ", originalTZ)
	}()

	for _, test := range []struct {
		name        string
		fixtureFile string
		resultFile  string
	}{
		{
			name:        "collectorHTML.tmpl",
			fixtureFile: "fixtures/status_info.json",
			resultFile:  "fixtures/status_info.html",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			jsonBytes, err := os.ReadFile(test.fixtureFile)
			require.NoError(t, err)
			var data map[string]interface{}
			err = json.Unmarshal(jsonBytes, &data)
			require.NoError(t, err)

			output := new(bytes.Buffer)
			err = status.RenderHTML(templatesFS, "collectorHTML.tmpl", output, data)
			require.NoError(t, err, "failed to render HTML")

			expectedOutput, err := os.ReadFile(test.resultFile)
			require.NoError(t, err)

			// We replace windows line break by linux so the tests pass on every OS
			result := strings.Replace(string(expectedOutput), "\r\n", "\n", -1)
			stringOutput := strings.Replace(output.String(), "\r\n", "\n", -1)

			require.Equal(t, result, stringOutput, "HTML rendering is not as expected")
		})
	}
}
