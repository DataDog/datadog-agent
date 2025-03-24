// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package diagnoseimpl

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
)

const runSuitetextresult = `=== Starting diagnose ===
==============
Suite: check-datadog
1. --------------
  UNEXPECTED ERROR [check] test
  Diagnosis: Check Dianose failes with unexpected errors
  Error: because it fails

2. --------------
  PASS [check] test 2
  Diagnosis: test 2 is working as expected

3. --------------
  WARNING [check] test 3
  Diagnosis: test 3 is not working as expected
  Remediation: restart the service

-------------------------
  Total:3, Success:1, Warning:1, Error:1
`

const runSuitejsonresult = `{
  "runs": [
    {
      "suite_name": "check-datadog",
      "diagnoses": [
        {
          "result": 3,
          "name": "test",
          "diagnosis": "Check Dianose failes with unexpected errors",
          "category": "check",
          "rawerror": "because it fails",
          "connectivity_result": "UNEXPECTED ERROR"
        },
        {
          "result": 0,
          "name": "test 2",
          "diagnosis": "test 2 is working as expected",
          "category": "check",
          "connectivity_result": "PASS"
        },
        {
          "result": 2,
          "name": "test 3",
          "diagnosis": "test 3 is not working as expected",
          "category": "check",
          "remediation": "restart the service",
          "connectivity_result": "WARNING"
        }
      ]
    }
  ],
  "summary": {
    "total": 3,
    "success": 1,
    "warnings": 1,
    "unexpected_error": 1
  }
}
`

func TestRunSuites(t *testing.T) {
	assert := assert.New(t)

	provides, err := NewComponent(Requires{})

	assert.Nil(err)

	setupDiagonseSuites(t)

	result, err := provides.Comp.RunSuites("text", true)
	assert.Nil(err)

	// We replace windows line break by linux so the tests pass on every OS
	expectedResult := strings.Replace(runSuitetextresult, "\r\n", "\n", -1)
	output := strings.Replace(string(result), "\r\n", "\n", -1)

	assert.Equal(expectedResult, output)

	result, err = provides.Comp.RunSuites("json", true)
	assert.Nil(err)

	// We replace windows line break by linux so the tests pass on every OS
	expectedResult = strings.Replace(runSuitejsonresult, "\r\n", "\n", -1)
	output = strings.Replace(string(result), "\r\n", "\n", -1)

	assert.Equal(expectedResult, output)
}

func TestRunSuite(t *testing.T) {
	assert := assert.New(t)

	provides, err := NewComponent(Requires{})

	assert.Nil(err)

	setupDiagonseSuites(t)

	result, err := provides.Comp.RunSuite(diagnose.CheckDatadog, "text", true)
	assert.Nil(err)

	// We replace windows line break by linux so the tests pass on every OS
	expectedResult := strings.Replace(runSuitetextresult, "\r\n", "\n", -1)
	output := strings.Replace(string(result), "\r\n", "\n", -1)

	assert.Equal(expectedResult, output)

	result, err = provides.Comp.RunSuite(diagnose.CheckDatadog, "json", true)
	assert.Nil(err)

	// We replace windows line break by linux so the tests pass on every OS
	expectedResult = strings.Replace(runSuitejsonresult, "\r\n", "\n", -1)
	output = strings.Replace(string(result), "\r\n", "\n", -1)

	assert.Equal(expectedResult, output)

	result, err = provides.Comp.RunSuite("non-existing", "json", true)
	assert.Error(err)

	assert.Equal("", string(result))
}

func TestAPIDiagnose(t *testing.T) {
	assert := assert.New(t)

	provides, err := NewComponent(Requires{})

	assert.Nil(err)

	setupDiagonseSuites(t)

	endpoint := provides.APIDiagnose.Provider.HandlerFunc()

	var cfgSer []byte
	cfgSer, err = json.Marshal(diagnose.Config{Verbose: true})
	assert.Nil(err)

	request := httptest.NewRequest("POST", "/diagnose", bytes.NewBuffer(cfgSer))
	response := httptest.NewRecorder()

	endpoint(response, request)

	assert.Equal(200, response.Code)

	var prettyJSON bytes.Buffer
	err = json.Indent(&prettyJSON, response.Body.Bytes(), "", "  ")
	assert.Nil(err)

	// We replace windows line break by linux so the tests pass on every OS
	expectedResult := strings.Replace(runSuitejsonresult, "\r\n", "\n", -1)
	output := strings.Replace(prettyJSON.String(), "\r\n", "\n", -1)

	assert.Equal(expectedResult, output)
}

func TestFlareProvider(t *testing.T) {
	assert := assert.New(t)

	provides, err := NewComponent(Requires{})

	assert.Nil(err)

	setupDiagonseSuites(t)

	flarecallback := provides.FlareProvider.FlareFiller.Callback

	mock := flarehelpers.NewFlareBuilderMock(t, false)

	flarecallback(mock)

	expectedResult := strings.Replace(runSuitetextresult, "\r\n", "\n", -1)

	mock.AssertFileContent(strings.TrimSuffix(expectedResult, "\n"), "diagnose.log")
}

func setupDiagonseSuites(t *testing.T) {
	t.Helper()

	diagnoseCatalog := diagnose.GetCatalog()

	oldSuites := diagnoseCatalog.Suites

	diagnoseCatalog.Suites = make(diagnose.Suites)

	diagnoseCatalog.Register(diagnose.CheckDatadog, func(_ diagnose.Config) []diagnose.Diagnosis {
		return []diagnose.Diagnosis{
			{
				Status:    diagnose.DiagnosisUnexpectedError,
				Name:      "test",
				Category:  "check",
				Diagnosis: "Check Dianose failes with unexpected errors",
				RawError:  "because it fails",
			},
			{
				Status:    diagnose.DiagnosisSuccess,
				Name:      "test 2",
				Category:  "check",
				Diagnosis: "test 2 is working as expected",
			},
			{
				Status:      diagnose.DiagnosisWarning,
				Name:        "test 3",
				Category:    "check",
				Diagnosis:   "test 3 is not working as expected",
				Remediation: "restart the service",
			},
		}
	})

	t.Cleanup(func() {
		diagnoseCatalog.Suites = oldSuites
	})
}
