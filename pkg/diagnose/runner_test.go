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

func TestDiagnoseSortingOrder(t *testing.T) {
	suitesDiagnoses := []diagnosis.Diagnoses{
		{
			SuiteName: "SN-single category",
			SuiteDiagnoses: []diagnosis.Diagnosis{
				{Name: "N3", Category: "C1"},
				{Name: "N1", Category: "C1"},
				{Name: "N2", Category: "C1"},
				{Name: "N4", Category: "C1"},
			},
		},
		{
			SuiteName: "SN-multiple categories",
			SuiteDiagnoses: []diagnosis.Diagnosis{
				{Name: "N3", Category: "C2"},
				{Name: "N1", Category: "C1"},
				{Name: "N2", Category: "C2"},
				{Name: "N4", Category: "C1"},
			},
		},
		{
			SuiteName: "SN-empty categories",
			SuiteDiagnoses: []diagnosis.Diagnosis{
				{Name: "N3"},
				{Name: "N1"},
				{Name: "N2"},
				{Name: "N4"},
			},
		},
		{
			SuiteName: "SN-helf-empty categories",
			SuiteDiagnoses: []diagnosis.Diagnosis{
				{Name: "N3"},
				{Name: "N1"},
				{Name: "N2", Category: "C2"},
				{Name: "N4", Category: "C1"},
			},
		},
		{
			SuiteName: "SN-half-empty names",
			SuiteDiagnoses: []diagnosis.Diagnosis{
				{Name: "N3", Category: "C1"},
				{Category: "C1"},
				{Name: "N2", Category: "C1"},
				{Category: "C1"},
			},
		},
	}

	expectedSuitesDiagnoses := []diagnosis.Diagnoses{
		{
			SuiteName: "SN-single category",
			SuiteDiagnoses: []diagnosis.Diagnosis{
				{Name: "N1", Category: "C1"},
				{Name: "N2", Category: "C1"},
				{Name: "N3", Category: "C1"},
				{Name: "N4", Category: "C1"},
			},
		},
		{
			SuiteName: "SN-multiple categories",
			SuiteDiagnoses: []diagnosis.Diagnosis{
				{Name: "N1", Category: "C1"},
				{Name: "N4", Category: "C1"},
				{Name: "N2", Category: "C2"},
				{Name: "N3", Category: "C2"},
			},
		},
		{
			SuiteName: "SN-empty categories",
			SuiteDiagnoses: []diagnosis.Diagnosis{
				{Name: "N1"},
				{Name: "N2"},
				{Name: "N3"},
				{Name: "N4"},
			},
		},
		{
			SuiteName: "SN-helf-empty categories",
			SuiteDiagnoses: []diagnosis.Diagnosis{
				{Name: "N1"},
				{Name: "N3"},
				{Name: "N4", Category: "C1"},
				{Name: "N2", Category: "C2"},
			},
		},
		{
			SuiteName: "SN-half-empty names",
			SuiteDiagnoses: []diagnosis.Diagnosis{
				{Category: "C1"},
				{Category: "C1"},
				{Name: "N2", Category: "C1"},
				{Name: "N3", Category: "C1"},
			},
		},
	}

	sortDiagnoses(suitesDiagnoses)

	// Enumberate suites
	for i, sd := range suitesDiagnoses {
		// Enumberate diagnoses
		for j, d := range sd.SuiteDiagnoses {
			assert.Equal(t, d.Category, expectedSuitesDiagnoses[i].SuiteDiagnoses[j].Category)
			assert.Equal(t, d.Name, expectedSuitesDiagnoses[i].SuiteDiagnoses[j].Name)
		}
	}
}
