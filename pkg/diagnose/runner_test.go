// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnose

import (
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"

	"github.com/stretchr/testify/assert"
)

func TestDiagnoseAllBasicRegAndRunNoDiagnoses(t *testing.T) {

	diagnosis.Register("TestDiagnoseAllBasicRegAndRunNoDiagnoses", func(cfg diagnosis.Config) []diagnosis.Diagnosis {
		return nil
	})

	re, _ := regexp.Compile("TestDiagnoseAllBasicRegAndRunNoDiagnoses")
	diagCfg := diagnosis.Config{
		Include:  []*regexp.Regexp{re},
		RunLocal: true,
	}
	diagnoses, err := Run(diagCfg)
	assert.NoError(t, err)
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
			RawError:    "Error_foo",
		},
		{
			Result:      diagnosis.DiagnosisFail,
			Name:        "Name_bar",
			Category:    "Category_bar",
			Description: "Description_bar",
			Remediation: "Remediation_bar",
			RawError:    "Error_bar",
		},
	}

	diagnosis.Register("TestDiagnoseAllBasicRegAndRunSomeDiagnosis-a", func(cfg diagnosis.Config) []diagnosis.Diagnosis {
		return inDiagnoses
	})

	diagnosis.Register("TestDiagnoseAllBasicRegAndRunSomeDiagnosis-b", func(cfg diagnosis.Config) []diagnosis.Diagnosis {
		return inDiagnoses
	})

	// Include and run
	reInclude, _ := regexp.Compile("TestDiagnoseAllBasicRegAndRunSomeDiagnosis")
	diagCfgInclude := diagnosis.Config{
		Include:  []*regexp.Regexp{reInclude},
		RunLocal: true,
	}
	outSuitesDiagnosesInclude, err := Run(diagCfgInclude)
	assert.NoError(t, err)
	assert.Len(t, outSuitesDiagnosesInclude, 2)
	assert.Equal(t, outSuitesDiagnosesInclude[0].SuiteDiagnoses, inDiagnoses)
	assert.Equal(t, outSuitesDiagnosesInclude[1].SuiteDiagnoses, inDiagnoses)

	// Include and Exclude and run
	reExclude, _ := regexp.Compile("TestDiagnoseAllBasicRegAndRunSomeDiagnosis-a")
	diagCfgIncludeExclude := diagnosis.Config{
		Include:  []*regexp.Regexp{reInclude},
		Exclude:  []*regexp.Regexp{reExclude},
		RunLocal: true,
	}
	outSuitesDiagnosesIncludeExclude, err := Run(diagCfgIncludeExclude)
	assert.NoError(t, err)
	assert.Len(t, outSuitesDiagnosesIncludeExclude, 1)
	assert.Equal(t, outSuitesDiagnosesIncludeExclude[0].SuiteDiagnoses, inDiagnoses)
	assert.Equal(t, outSuitesDiagnosesIncludeExclude[0].SuiteName, "TestDiagnoseAllBasicRegAndRunSomeDiagnosis-b")
}
