// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnose

import (
	"bytes"
	"errors"
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"

	"github.com/stretchr/testify/assert"
)

func TestConnectivityAutodiscovery(t *testing.T) {

	diagnosis.RegisterMetadataAvail("failing", func() error { return errors.New("fail") })
	diagnosis.RegisterMetadataAvail("succeeding", func() error { return nil })

	w := &bytes.Buffer{}
	RunMetadataAvail(w)

	result := w.String()
	assert.Contains(t, result, "=== Running failing diagnosis ===\n===> FAIL")
	assert.Contains(t, result, "=== Running succeeding diagnosis ===\n===> PASS")
}

func TestDiagnoseAllBasicRegAndRunNoDiagnoses(t *testing.T) {

	diagnosis.Register("TestDiagnoseAllBasicRegAndRunNoDiagnoses", func(cfg diagnosis.DiagnoseConfig) []diagnosis.Diagnosis {
		return nil
	})

	re, _ := regexp.Compile("TestDiagnoseAllBasicRegAndRunNoDiagnoses")
	diagCfg := diagnosis.DiagnoseConfig{
		Include: []*regexp.Regexp{re},
	}
	diagnoses := RunAll(diagCfg)
	assert.Len(t, diagnoses, 0)
}

func TestDiagnoseAllBasicRegAndRunSomeDiagnosis(t *testing.T) {

	inDiagnoses := []diagnosis.Diagnosis{
		{
			Result:      diagnosis.DiagnosisSuccess,
			Name:        "Name_foo",
			Category:    "Category_foo",
			Description: "Description_foo",
			Remediation: "Remediation_foo",
			RawError:    errors.New("Error_foo"),
		},
		{
			Result:      diagnosis.DiagnosisFail,
			Name:        "Name_bar",
			Category:    "Category_bar",
			Description: "Description_bar",
			Remediation: "Remediation_bar",
			RawError:    errors.New("Error_bar"),
		},
	}

	diagnosis.Register("TestDiagnoseAllBasicRegAndRunSingleDiagnosis", func(cfg diagnosis.DiagnoseConfig) []diagnosis.Diagnosis {
		return inDiagnoses
	})

	re, _ := regexp.Compile("TestDiagnoseAllBasicRegAndRunSingleDiagnosis")
	diagCfg := diagnosis.DiagnoseConfig{
		Include: []*regexp.Regexp{re},
	}
	outSuitesDiagnoses := RunAll(diagCfg)
	assert.Len(t, outSuitesDiagnoses, 1)
	assert.Equal(t, outSuitesDiagnoses[0].SuiteDiagnoses, inDiagnoses)
}
